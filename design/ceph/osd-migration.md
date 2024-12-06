# OSD Migration

## Summary

- Automatically replace OSDs in scenarios where the configuration change does not
  allow the OSDs to be upgraded.

## Motivation:

- Certain scenarios like enabling encryption on existing OSDs or changing the OSD
  backend store require the users to manually purge the OSDs and then create new ones.
  Rook should be able to migrate the OSDs automatically without any user intervention.

## Goals

- Support automatic OSD migration for following scenarios:
    - Change OSD backing store
    - Enable or Disable OSD encryption as a day-2 operation

## Non-Goals/Deferred Features:

Migration of OSDs with following configurations is deferred for now and will
considered in the future:
- Hybrid OSDs where metadata (RocksDB+WAL) is placed on faster storage media
  and application data on slower media.
- Setups with multiple OSDs per drive, though with recent Ceph releases the
  motivation for deploying this way is mostly obviated.
- OSDs where Persistent Volumes are using partitioned disks due to a [ceph issue](https://tracker.ceph.com/issues/68977). 

## Proposal
- Since migration requires destroying of the OSD and cleaning data from the disk,
  a user confirmation in the CR is required before proceeding.
- Operator would identify the OSDs that need migration according to the updated
  configuration and existing OSD deployment labels
- Operator would display the status of OSD migration in the cephCluster CR status.

### API Changes
- Add `spec.storage.migration` in the CephCluster resource.

  ```yaml
  storage:
    migration:
        confirmation: yes-really-migrate-osds
  ```
  - `confirmation`: Confirmation from the user that they really want to migrate the OSDs.
    This field can only take the value `yes-really-migrate-osds`.


### Enable Encryption
- Storage spec for a brownfield cluster without encryption:
```yaml
storage:
   storageClassDeviceSets:
     - name: set1
       count: 3
       encrypted: false
```
- User can enable encryption by updating the spec:
``` yaml
 storage:
   migration:
    confirmation: "yes-really-migrate-osds"
   storageClassDeviceSets:
     - name: set1
       count: 3
       encrypted: true  # changed to true
```

- Operator will check the OSD deployment `encrypted: false` to identify the OSDs that are not encrypted.
- Operator will start the migration for these OSDs.

### Update OSD backend Store:
- Storage spec for a brownfield cluster without encryption would look like:
```yaml
storage:
   store:
     type: bluestore
```
- User can change backend store by updating the spec as below:
``` yaml
 storage:
   migration:
    confirmation: "yes-really-migrate-osds"
   store:
     type: <new-backend-store>  # changed to new backend store type (such as seastore in the future)
```

- Operator will check the OSD deployment label `osd-store` to identify the OSDs that have different backend store.
- Operator will start the migration for these OSDs.

### Migrate OSDs for Multiple Scenarios
- Users can update multiple settings in the cephCluster CR at the same time. For example, enable encryption and change backing store.
- The operator would migrate the OSDs and apply all the new settings.

### Migration Status:
- Operator will store the OSD migration status info in `status.storage.osd`
  ``` yaml
  status:
   storage:
      osd:
       migrationStatus:
        pending: 5
  ```
  - `osd.migrationStatus.pending`: Total number of OSDs that are pending migration.
  - The cluster's `phase` should be set to `progressing` while OSDs are migrating


### Automated OSD Migration Process
- The operator's reconciler will migrate one OSD at a time.
- A configmap will be used to store the OSD ID currently being migrated.

OSD replacement steps:

1. List all OSDs where `osd-store:<osd store type>` does not match `spec.storage.store.type`.
1. If all PGs are not `active+clean`, do not proceed.
1. If all PGs are `active+clean` but a previous OSD migration is not completed, do not proceed.
1. If all PGs are `active+clean` and no migration is in progress, then select an OSD to be migrated.
1. Delete the OSD deployment.
1. Create an OSD prepare job with an environment variable indictating the OSD ID to be replaced.
1. The OSD prepare pod will destroy the OSD (`ceph osd destroy {id} --yes-i-really-mean-it`) and prepare it again using the same OSD ID. Refer [Destroy OSD](#destroy-osd) for details.
1. Once the destroyed OSD pod is recreated, delete the configmap.
1. If there is any error during the OSD migration, then preserve the OSD ID being replaced in the configmap for next reconcile.
1. Reconcile the operator and perform as same steps until all the OSDs have migrated to the new backend.


## Risks
- OSD migration involves destroying OSD and cleaning up backing disk. So I/O performance will be impacted
  during OSD migration as some bandwidth will be utilized for backfilling.
