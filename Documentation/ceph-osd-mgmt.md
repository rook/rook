---
title: Ceph OSD Management
weight: 10610
indent: true
---

# Ceph OSD Management

Ceph Object Storage Daemons (OSDs) are the heart and soul of the Ceph storage platform.
Each OSD manages a local device and together they provide the distributed storage. Rook will automate creation and management of OSDs to hide the complexity
based on the desired state in the CephCluster CR as much as possible. This guide will walk through some of the scenarios
to configure OSDs where more configuration may be required.

## OSD Health

The [rook-ceph-tools pod](./ceph-toolbox.md) provides a simple environment to run Ceph tools. The `ceph` commands
mentioned in this document should be run from the toolbox.

Once the is created, connect to the pod to execute the `ceph` commands to analyze the health of the cluster,
in particular the OSDs and placement groups (PGs). Some common commands to analyze OSDs include:
```
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

The [QuickStart Guide](ceph-quickstart.md) will provide the basic steps to create a cluster and start some OSDs. For more details on the OSD
settings also see the [Cluster CRD](ceph-cluster-crd.md) documentation. If you are not seeing OSDs created, see the [Ceph Troubleshooting Guide](ceph-common-issues.md).

To add more OSDs, Rook will automatically watch for new nodes and devices being added to your cluster.
If they match the filters or other settings in the `storage` section of the cluster CR, the operator
will create new OSDs.

## Add an OSD on a PVC

In more dynamic environments where storage can be dynamically provisioned with a raw block storage provider, the OSDs can be backed
by PVCs. See the `storageClassDeviceSets` documentation in the [Cluster CRD](ceph-cluster-crd.md#storage-class-device-sets) topic.

To add more OSDs, you can either increase the `count` of the OSDs in an existing device set or you can
add more device sets to the cluster CR. The operator will then automatically create new OSDs according
to the updated cluster CR.

## Remove an OSD

Removal of OSDs is intentionally not automated. Rook's charter is to keep your data safe, not to delete it. If you are
sure you need to remove OSDs, it can be done. We just want you to be in control of this action.

To remove an OSD due to a failed disk or other re-configuration, consider the following to ensure the health of the data
through the removal process:
- Confirm you will have enough space on your cluster after removing your OSDs to properly handle the deletion
- Confirm the remaining OSDs and their placement groups (PGs) are healthy in order to handle the rebalancing of the data
- Do not remove too many OSDs at once
- Wait for rebalancing between removing multiple OSDs

If all the PGs are `active+clean` and there are no warnings about being low on space, this means the data is fully replicated
and it is safe to proceed. If an OSD is failing, the PGs will not be perfectly clean and you will need to proceed anyway.

### From the Toolbox

1. Determine the OSD ID for the OSD to be removed. The osd pod may be in an error state such as `CrashLoopBackoff` or the `ceph` commands
in the toolbox may show which OSD is `down`.
2. Mark the OSD as `out` if not already marked as such by Ceph. This signals Ceph to start moving (backfilling) the data that was on that OSD to another OSD.
   - `ceph osd out osd.<ID>` (for example if the OSD ID is 23 this would be `ceph osd out osd.23`)
3. Wait for the data to finish backfilling to other OSDs.
   - `ceph status` will indicate the backfilling is done when all of the PGs are `active+clean`.
4. Remove the disk from the node.
5. Update your CephCluster CR such that the operator won't create an OSD on the device anymore.
Depending on your CR settings, you may need to remove the device from the list or update the device filter.
If you are using `useAllDevices: true`, no change to the CR is necessary.
6. Remove the OSD from the Ceph cluster
   - `ceph osd purge <ID> --yes-i-really-mean-it`
7. Verify the OSD is removed from the node in the CRUSH map
   - `ceph osd tree`

### Remove the OSD Deployment

The operator can automatically remove OSD deployments that are considered "safe-to-destroy" by Ceph.
After the steps above, the OSD will be considered safe to remove since the data has all been moved
to other OSDs. But this will only be done automatically by the operator if you have this setting in the cluster CR:
```
removeOSDsIfOutAndSafeToRemove: true
```

8. Otherwise, you will need to delete the deployment directly:
   - `kubectl delete deployment -n rook-ceph rook-ceph-osd-<ID>`

### Delete the underlying data

9. If you want to clean the device where the OSD was running, see in the instructions to
wipe a disk on the [Cleaning up a Cluster](ceph-teardown.md#delete-the-data-on-hosts) topic.

## Replace an OSD

To replace a disk that has failed:

1. Run the steps in the previous section to [Remove an OSD](#remove-an-osd).
2. Replace the physical device and verify the new device is attached.
3. Check if your cluster CR will find the new device. If you are using `useAllDevices: true` you can skip this step.
If your cluster CR lists individual devices or uses a device filter you may need to update the CR.
4. The operator ideally will automatically create the new OSD within a few minutes of adding the new device or updating the CR.
If you don't see a new OSD automatically created, restart the operator (by deleting the operator pod) to trigger the OSD creation.
5. Verify if the OSD is created on the node by running `ceph osd tree` from the toolbox.

Note that the OSD might have a different ID than the previous OSD that was replaced.

## Remove an OSD from a PVC

If you have installed your OSDs on top of PVCs and you desire to reduce the size of your cluster by removing OSDs:

1. Shrink the number of OSDs in the `storageClassDeviceSet` in the CephCluster CR.
   - `kubectl -n rook-ceph edit cephcluster rook-ceph`
   - Reduce the `count` of the OSDs to the desired number. Rook will not take any action to automatically remove the extra OSD(s), but will effectively stop managing the orphaned OSD.
2. Identify the orphaned PVC that belongs to the orphaned OSD.
   - The orphaned PVC will have the highest index among the PVCs for the device set.
   - `kubectl -n rook-ceph get pvc -l ceph.rook.io/DeviceSet=<deviceSet>`
   - For example if the device set is named `set1` and the `count` was reduced from `3` to `2`, the orphaned PVC would have the index `2` and might be named `set1-2-data-vbwcf`
3. Identify the orphaned OSD.
   - The OSD assigned to the PVC can be found in the labels on the PVC
   - `kubectl -n rook-ceph get pod -l ceph.rook.io/pvc=<orphaned-pvc> -o yaml | grep ceph-osd-id`
   - For example, this might return: `ceph-osd-id: "0"`
4. Now proceed with the steps in the section above to [Remove an OSD](#remove-an-osd) for the orphaned OSD ID.
5. If desired, delete the orphaned PVC after the OSD is removed.
