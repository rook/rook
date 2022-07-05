---
title: Ceph OSD Management
---

Ceph Object Storage Daemons (OSDs) are the heart and soul of the Ceph storage platform.
Each OSD manages a local device and together they provide the distributed storage. Rook will automate creation and management of OSDs to hide the complexity
based on the desired state in the CephCluster CR as much as possible. This guide will walk through some of the scenarios
to configure OSDs where more configuration may be required.

## OSD Health

The [rook-ceph-tools pod](../../Troubleshooting/ceph-toolbox.md) provides a simple environment to run Ceph tools. The `ceph` commands
mentioned in this document should be run from the toolbox.

Once the is created, connect to the pod to execute the `ceph` commands to analyze the health of the cluster,
in particular the OSDs and placement groups (PGs). Some common commands to analyze OSDs include:

```console
ceph status
ceph osd tree
ceph osd status
ceph osd df
ceph osd utilization
```

```console
kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}') bash
```

## Add an OSD

The [QuickStart Guide](../../Getting-Started/quickstart.md) will provide the basic steps to create a cluster and start some OSDs. For more details on the OSD
settings also see the [Cluster CRD](../../CRDs/Cluster/ceph-cluster-crd.md) documentation. If you are not seeing OSDs created, see the [Ceph Troubleshooting Guide](../../Troubleshooting/ceph-common-issues.md).

To add more OSDs, Rook will automatically watch for new nodes and devices being added to your cluster.
If they match the filters or other settings in the `storage` section of the cluster CR, the operator
will create new OSDs.

## Add an OSD on a PVC

In more dynamic environments where storage can be dynamically provisioned with a raw block storage provider, the OSDs can be backed
by PVCs. See the `storageClassDeviceSets` documentation in the [Cluster CRD](../../CRDs/Cluster/ceph-cluster-crd.md#storage-class-device-sets) topic.

To add more OSDs, you can either increase the `count` of the OSDs in an existing device set or you can
add more device sets to the cluster CR. The operator will then automatically create new OSDs according
to the updated cluster CR.

## Remove an OSD

To remove an OSD due to a failed disk or other re-configuration, consider the following to ensure the health of the data
through the removal process:

* Confirm you will have enough space on your cluster after removing your OSDs to properly handle the deletion
* Confirm the remaining OSDs and their placement groups (PGs) are healthy in order to handle the rebalancing of the data
* Do not remove too many OSDs at once
* Wait for rebalancing between removing multiple OSDs

If all the PGs are `active+clean` and there are no warnings about being low on space, this means the data is fully replicated
and it is safe to proceed. If an OSD is failing, the PGs will not be perfectly clean and you will need to proceed anyway.

### Host-based cluster

Update your CephCluster CR. Depending on your CR settings, you may need to remove the device from the list or update the device filter.
If you are using `useAllDevices: true`, no change to the CR is necessary.

!!! important
    **On host-based clusters, you may need to stop the Rook Operator while performing OSD
    removal steps in order to prevent Rook from detecting the old OSD and trying to re-create it before the disk is wiped or removed.**

To stop the Rook Operator, run:

```console
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=0
```

You must perform steps below to (1) purge the OSD and either (2.a) delete the underlying data or
(2.b)replace the disk before starting the Rook Operator again.

Once you have done that, you can start the Rook operator again with:

```console
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=1
```

### PVC-based cluster

To reduce the storage in your cluster or remove a failed OSD on a PVC:

1. Shrink the number of OSDs in the `storageClassDeviceSets` in the CephCluster CR. If you have multiple device sets,
    you may need to change the index of `0` in this example path.
    * `kubectl -n rook-ceph patch CephCluster rook-ceph --type=json -p '[{"op": "replace", "path": "/spec/storage/storageClassDeviceSets/0/count", "value":<desired number>}]'`
    * Reduce the `count` of the OSDs to the desired number. Rook will not take any action to automatically remove the extra OSD(s).
2. Identify the PVC that belongs to the OSD that is failed or otherwise being removed.
    * `kubectl -n rook-ceph get pvc -l ceph.rook.io/DeviceSet=<deviceSet>`
3. Identify the OSD you desire to remove.
    * The OSD assigned to the PVC can be found in the labels on the PVC
    * `kubectl -n rook-ceph get pod -l ceph.rook.io/pvc=<orphaned-pvc> -o yaml | grep ceph-osd-id`
    * For example, this might return: `ceph-osd-id: "0"`
    * Remember the OSD ID for purging the OSD below

If you later increase the count in the device set, note that the operator will create PVCs with the highest index
that is not currently in use by existing OSD PVCs.

### Confirm the OSD is down

If you want to remove an unhealthy OSD, the osd pod may be in an error state such as `CrashLoopBackoff` or the `ceph` commands
in the toolbox may show which OSD is `down`. If you want to remove a healthy OSD, you should run the following commands:

```console
$ kubectl -n rook-ceph scale deployment rook-ceph-osd-<ID> --replicas=0
# Inside the toolbox
$ ceph osd down osd.<ID>
```

### Purge the OSD from the Ceph cluster

OSD removal can be automated with the example found in the [rook-ceph-purge-osd job](https://github.com/rook/rook/blob/master/deploy/examples/osd-purge.yaml).
In the osd-purge.yaml, change the `<OSD-IDs>` to the ID(s) of the OSDs you want to remove.

1. Run the job: `kubectl create -f osd-purge.yaml`
2. When the job is completed, review the logs to ensure success: `kubectl -n rook-ceph logs -l app=rook-ceph-purge-osd`
3. When finished, you can delete the job: `kubectl delete -f osd-purge.yaml`

If you want to remove OSDs by hand, continue with the following sections. However, we recommend you to use the above-mentioned job to avoid operation errors.

### Purge the OSD manually

If the OSD purge job fails or you need fine-grained control of the removal, here are the individual commands that can be run from the toolbox.

1. Detach the OSD PVC from Rook
    * `kubectl -n rook-ceph label pvc <orphaned-pvc> ceph.rook.io/DeviceSetPVCId-`
2. Mark the OSD as `out` if not already marked as such by Ceph. This signals Ceph to start moving (backfilling) the data that was on that OSD to another OSD.
    * `ceph osd out osd.<ID>` (for example if the OSD ID is 23 this would be `ceph osd out osd.23`)
3. Wait for the data to finish backfilling to other OSDs.
    * `ceph status` will indicate the backfilling is done when all of the PGs are `active+clean`. If desired, it's safe to remove the disk after that.
4. Remove the OSD from the Ceph cluster
    * `ceph osd purge <ID> --yes-i-really-mean-it`
5. Verify the OSD is removed from the node in the CRUSH map
    * `ceph osd tree`

The operator can automatically remove OSD deployments that are considered "safe-to-destroy" by Ceph.
After the steps above, the OSD will be considered safe to remove since the data has all been moved
to other OSDs. But this will only be done automatically by the operator if you have this setting in the cluster CR:

```yaml
removeOSDsIfOutAndSafeToRemove: true
```

Otherwise, you will need to delete the deployment directly:

```console
kubectl delete deployment -n rook-ceph rook-ceph-osd-<ID>
```

In PVC-based cluster, remove the orphaned PVC, if necessary.

### Delete the underlying data

If you want to clean the device where the OSD was running, see in the instructions to
wipe a disk on the [Cleaning up a Cluster](../ceph-teardown.md#delete-the-data-on-hosts) topic.

## Replace an OSD

To replace a disk that has failed:

1. Run the steps in the previous section to [Remove an OSD](#remove-an-osd).
2. Replace the physical device and verify the new device is attached.
3. Check if your cluster CR will find the new device. If you are using `useAllDevices: true` you can skip this step.
If your cluster CR lists individual devices or uses a device filter you may need to update the CR.
4. The operator ideally will automatically create the new OSD within a few minutes of adding the new device or updating the CR.
If you don't see a new OSD automatically created, restart the operator (by deleting the operator pod) to trigger the OSD creation.
5. Verify if the OSD is created on the node by running `ceph osd tree` from the toolbox.

!!! note
    The OSD might have a different ID than the previous OSD that was replaced.
