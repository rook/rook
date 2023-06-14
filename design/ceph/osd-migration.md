# OSD Backend Store Migration

## Summary

- Configure OSD backend store via `cephCluster` CR so that new OSDs can use it.
- Existing OSDs should be able to migrate to use a new backend store configured via `cephCluster` CR. The migration process should include destroying the existing OSDs one by one, wiping the disk and then creating a new OSD on that disk.

## Motivation:

- Ceph uses BlueStore as a special-purpose storage back end designed specifically for managing data on disk for Ceph OSD workloads. Ceph team is working on improving this backend storage that will help in significantly improving the performance, especially in disaster recovery scenarios. This new OSD backend is known as `bluestore-rdr`. In the future, other backends will also need to be supported, such as `seastore`.
- To support any new and improved storage backend, the existing OSD needs to be destroyed, disks should be wiped out and new OSD should be created using the latest storage backend.

## Goals

- Newly provisioned clusters should be able to use the specific storage backend while creating OSDs.
- Existing clusters should be able to migrate the OSDs to use a specific storage backend, without downtime and without risking data loss.

## Non-Goals/Deferred Features:

Migration of OSDs with following configurations is deferred for now and will be taken up in future:
- OSDs on mixed media setup where metadata (RocksDB+WAL) is placed on higher throughput storage media and application data on lower throughput media.
- Setups with multiple OSDs per disk.

## Proposal

### API Changes
- Add `spec.storage.store` in the ceph cluster yaml.

  ```yaml
  storage:
    store:
      type: bluestore-rdr
      updateStore: yes-really-update-store
  ```

  - `type`: The type of backend store to be used for OSDs. For example, bluestore, bluestore-rdr, etc. The default store type will be `bluestore`
  - `updateStore`: Allows the user to migrate the existing OSDs to use any new storage backend. This field can only take the value as `yes-really-update-store`. If the user wants to change the `store.type` field for an existing cluster, then they will also need to update `spec.storage.store.updateStore` with `yes-really-update-store`


- Add `status.storage.osd` to the ceph cluster status. This will help convey the overall status of the OSD migration

  ``` yaml
  status:
   storage:
      osd:
       storeType:
        bluestore: 3
        bluestore-rdr: 5
  ```

  - `storeType.bluestore`: Total number of bluestore OSDs running
  - `storeType.bluestore-rdr`: Total number of bluestore-rdr OSDs running
  - Cluster `phase` should be set to progressing while OSDs are migrating

The migration process will involve destroying the existing OSDs one by one, wiping the disks, creating a new OSD and waiting for the PGs to be active+clean before migrating the next OSD. Since this operation involves possible downtime, users should be really sure before proceeding with this action.

**NOTE**: Once the OSDs are migrated to a new backend store, say, `bluestore-rdr`, they won't be allowed to be migrated back to the legacy store (bluestore).

- Add a new label `osd-store:<osd store type>` to all the OSD pods.
- This label will help to identify the current backend store being used for the OSD and enable the operator to decide if the OSD should be migrated in case the user changes the backend store type in the spec (`spec.Storage.store.type`).

### New OSDs

- Add the OSD store provided by the user in `spec.storage.store.type` as an environment variable in the OSD prepare job. If no OSD store is provided in the spec, then set the environment variable to `bluestore`.
- The prepare pod will use this environment variable while preparing OSDs using `ceph-volume` command.

#### OSD Prepare

  - RAW MODE

    ```
    ceph-volume raw prepare <OSD_STORE_ENV_VARIABLE> --data /dev/vda
    ```

  - LVM MODE

    ```
    ceph-volume lvm prepare <OSD_STORE_ENV_VARIABLE> --data /dev/vda
    ```

#### OSD Activate

ceph-volume `activate` command doesn't require OSD store flag to be passed as an argument. It auto detects the backend store that was used during the OSD `prepare`

### Existing OSDs

- After upgrading an existing rook ceph cluster to a Ceph version that supports `bluestore-rdr`, users can upgrade the `bluestore` based OSDs to use `bluestore-rdr`
- Backend store of the OSDs can not be overridden, so this update will require the OSDs to be replaced one by one.
- In order to migrate OSDs to use `bluestore-rdr`, users need to patch the ceph cluster spec like below:

```yaml
storage:
  store:
    type: bluestore-rdr
    updateStore: yes-really-update-store
```

#### Replace OSD
Operator reconciler will replace one OSD at a time.
A config map will be used to store the OSD being replaced currently.
OSD replacement steps:
  1. List out all OSDs where `osd-store:<osd store type>` does not match with `spec.storage.store.type`.
  2. If the PG status is not `active+clean`, don't replace any OSD.
  3. If the PG status is `active+clean` but a previous OSD replacement is not completed, then don't replace any new OSD.
  4. If the PG status is `active+clean` and no replacement is in progress, then select an OSD to be replaced from the list.
  5. Delete the OSD deployment.
  6. Create an OSD prepare job with an environment variable of OSD ID to be replaced.
  7. OSD prepare pod will destroy the OSD and prepare it again using the same OSD ID. Refer [Destroy OSD](#destroy-osd) for details.
  8. Once the destroyed OSD pod is added back, delete the configmap.
  9. If there is any error during the OSD migration, then preserve the OSD being replaced in the configmap for next reconcile.
  10. Reconcile the operator and perform as same steps until all the OSDs have migrated to the new backend store.

#### Destroy OSD

OSD prepare pod job will destroy an OSD using following steps.
- Check for OSD ID environment variable of the OSD to be destroyed.
- Use `ceph volume list` to fetch the OSD path.
- Destroy the OSD using following command:

    ```
    ceph osd destroy <OSD_ID> --yes-i-really-mean-it
    ```

- Wipe the OSD disk. This removes all the data on the device.
- Prepare the OSD with the new store type by using the same OSD ID. This is done by passing the OSD ID as `--osd-id` flag to `ceph-volume` command.

    ```
    ceph-volume lvm prepare --osd-id <OSD_ID> --data <OSD_PATH>
    ```

## Planning

These changes require a significant developemnt effort to migrate existing OSDs to use new backend store. So it will be divided into following stages:
* New OSDs (greenfield).
* Migrating existing OSDs on PVC without metadata devices.
* Migrating existing OSDs on PVC with metadata devices.
* Existing node-based OSDs (multiple OSDs are created at once using ceph volume batch which adds to additional complications).
