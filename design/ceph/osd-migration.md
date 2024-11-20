<<<<<<< HEAD
# OSD Backend Store Migration

## Summary

- Configure the default OSD backend via the `cephCluster` CR so that new OSDs
  are built with it.
- Existing OSDs should be able to migrate to use a new backend store configured
  via `cephCluster` CR. The migration process should include destroying the
  existing OSDs one by one, wiping the drive, then recreating a new OSD on
  that drive with the same ID.

## Motivation:

- Ceph uses BlueStore as a special-purpose storage backend designed specifically
  for Ceph OSD workloads. The upstream Ceph team is working on improving
  BlueStore in ways that will significantly improve performance, especially
  in disaster recovery scenarios. This new OSD backend is known as
  `bluestore-rdr`. In the future, other backends will also need to be supported,
  such as `seastore` from the Crimson effort.
- To employ a new torage backend, the existing OSD needs to be destroyed, drives
  wiped, and new OSD created using the specified storage backend.

## Goals

- Newly provisioned clusters should be able to use the specific storage backend
  when creating OSDs.
- Existing clusters should be able to migrate existing OSDs to a specific
  backend, without downtime and without risking data loss.  Note that this
  necessarily entails redeploying and backfilling each OSD in turn.
=======
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
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

## Non-Goals/Deferred Features:

Migration of OSDs with following configurations is deferred for now and will
considered in the future:
- Hybrid OSDs where metadata (RocksDB+WAL) is placed on faster storage media
  and application data on slower media.
- Setups with multiple OSDs per drive, though with recent Ceph releases the
  motivation for deploying this way is mostly obviated.

## Proposal
<<<<<<< HEAD

### API Changes
- Add `spec.storage.store` in the Ceph cluster YAML.

  ```yaml
  storage:
    store:
      type: bluestore-rdr
      updateStore: yes-really-update-store
  ```

  - `type`: The backend to be used for OSDs: `bluestore`, `bluestore-rdr`,
     etc. The default type will be `bluestore`
  - `updateStore`: Allows the operator to migrate existing OSDs to a different
     backend. This field can only take the value `yes-really-update-store`. If
     the user wants to change the `store.type` field for an existing cluster,
     they will also need to update `spec.storage.store.updateStore` with `yes-really-update-store`.


- Add `status.storage.osd` to the Ceph cluster status. This will help convey the progress
  of OSD migration

=======
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
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
  ``` yaml
  status:
   storage:
      osd:
<<<<<<< HEAD
       storeType:
        bluestore: 3
        bluestore-rdr: 5
  ```

  - `storeType.bluestore`: Total number of BlueStore OSDs running
  - `storeType.bluestore-rdr`: Total number of BlueStore-rdr OSDs running
  - The cluster's `phase` should be set to `progressing` while OSDs are migrating

The migration process will involve destroying existing OSDs one by one, wiping
the drives, deploying a new OSD with the same ID, then waiting for all PGs
to be `active+clean` before migrating the next OSD. Since this operation
involves possible impact or downtime, users should be exercise caution
before proceeding with this action.

**NOTE**: Once the OSDs are migrated to a new backend, say `bluestore-rdr`, they
won't be allowed to be migrated back to the legacy store (BlueStore).

- Add a new label `osd-store:<osd store type>` to all OSD pods.
- This label will help to identify the current backend being used for the OSD
  and enable the operator to determine if the OSD should be migrated if the
  the admin changes the backend type in the spec (`spec.Storage.store.type`).

### New OSDs

- Set the OSD backend provided by the user in `spec.storage.store.type` as an
  environment variable in the OSD prepare job. If no OSD store is provided in
  the spec, then set the environment variable to `bluestore`.
- The prepare pod will use this environment variable when preparing OSDs
  with the `ceph-volume` command.

#### OSD Prepare

  - RAW MODE

    ```
    ceph-volume raw prepare <OSD_STORE_ENV_VARIABLE> --data /dev/vda
    ```

  - LVM MODE

    ```
    ceph-volume lvm prepare <OSD_STORE_ENV_VARIABLE> --data /dev/vda
    ```

#### OSD Activation

The ceph-volume `activate` command doesn't require the OSD backend to be passed
as an argument. It auto-detects the backend that was used during when the OSD
was prepared.

### Existing OSDs

- After upgrading an existing Rook Ceph cluster to a Ceph release that supports
  `bluestore-rdr`, admins can migrate `bluestore` OSDs to `bluestore-rdr`.
- The backend of OSDs can not be overridden, so this update will require
  the OSDs to be replaced one by one.
- In order to migrate OSDs to use `bluestore-rdr`, admins must patch the
  Ceph cluster spec as below:

```yaml
storage:
  store:
    type: bluestore-rdr
    updateStore: yes-really-update-store
```

#### Replace OSD
The operator's reconciler will replace one OSD at a time.
A configmap will be used to store the OSD ID currently being migrated.
OSD replacement steps:
  1. List all OSDs where `osd-store:<osd store type>` does not match `spec.storage.store.type`.
  2. If all PGs are not `active+clean`, do not proceed.
  3. If all PGs  are `active+clean` but a previous OSD replacement is not completed, do not proceed.
  4. If all PGs are `active+clean` and no replacement is in progress, then select an OSD to be migrated.
  5. Delete the OSD deployment.
  6. Create an OSD prepare job with an environment variable indictating the OSD ID to be replaced.
  7. The OSD prepare pod will destroy the OSD and prepare it again using the same OSD ID. Refer [Destroy OSD](#destroy-osd) for details.
  8. Once the destroyed OSD pod is recreated, delete the configmap.
  9. If there is any error during the OSD migration, then preserve the OSD ID being replaced in the configmap for next reconcile.
  10. Reconcile the operator and perform as same steps until all the OSDs have migrated to the new backend.

#### Destroy OSD

The OSD prepare pod job will destroy an OSD using following steps:
- Check for OSD ID environment variable of the OSD to be destroyed.
- Use `ceph volume list` to fetch the OSD path.
- Destroy the OSD using following command:

    ```
    ceph osd destroy <OSD_ID> --yes-i-really-mean-it
    ```

- Wipe the OSD drive. This removes all the data on the device.
- Prepare the OSD with the new store type by using the same OSD ID. This is done by passing the OSD ID as `--osd-id` flag to `ceph-volume` command.

    ```
    ceph-volume lvm prepare --osd-id <OSD_ID> --data <OSD_PATH>
    ```

## Planning

These changes require significant development effort to migrate existing OSDs to use a new backend.
They will be divided into following phases:
* New OSDs (greenfield).
* Migrating existing OSDs on PVC without metadata devices.
* Migrating existing OSDs on PVC with metadata devices.
* Existing node-based OSDs (multiple OSDs are created at once via `ceph-volume batch` which adds additional complications).
=======
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
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
