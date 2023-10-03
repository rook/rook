---
title: Cleanup
---

## Cleaning up a Cluster

To tear down the cluster, the following resources need to be cleaned up:

* The resources created under Rook's namespace (default `rook-ceph`) such as the Rook operator created by `operator.yaml` and the cluster CR `cluster.yaml`.
* `/var/lib/rook/rook-ceph`: Path on each host in the cluster where configuration is stored by the ceph mons and osds
* Devices used by the OSDs

If the default namespaces or paths such as `dataDirHostPath` are changed in the example yaml files, these namespaces and paths will need to be changed throughout these instructions.

If tearing down a cluster frequently for development purposes, it is instead recommended to use an environment such as [Minikube](../Contributing/development-environment.md) that can easily be reset without worrying about any of these steps.

### Delete the Block and File artifacts

First clean up the resources from applications that consume the Rook storage.

These commands will clean up the resources from the example application [block](../Storage-Configuration/Block-Storage-RBD/block-storage.md#teardown) and [file](../Storage-Configuration/Shared-Filesystem-CephFS/filesystem-storage.md#teardown) walkthroughs (unmount volumes, delete volume claims, etc).

```console
kubectl delete -f ../wordpress.yaml
kubectl delete -f ../mysql.yaml
kubectl delete -n rook-ceph cephblockpool replicapool
kubectl delete storageclass rook-ceph-block
kubectl delete -f csi/cephfs/kube-registry.yaml
kubectl delete storageclass csi-cephfs
```

!!! important
    After applications have been cleaned up, the Rook cluster can be removed. It is important to delete applications before removing the Rook operator and Ceph cluster. Otherwise, volumes may hang and nodes may require a restart.

### Delete the CephCluster CRD

!!! warning
    DATA WILL BE PERMANENTLY DELETED AFTER DELETING THE `CephCluster`

1. To instruct Rook to wipe the host paths and volumes, edit the `CephCluster` and add the `cleanupPolicy`:

```console
kubectl -n rook-ceph patch cephcluster rook-ceph --type merge -p '{"spec":{"cleanupPolicy":{"confirmation":"yes-really-destroy-data"}}}'
```

Once the cleanup policy is enabled, any new configuration changes in the CephCluster will be blocked. Nothing will happen until the deletion of the CR is requested, so this `cleanupPolicy` change can still be reverted if needed.

Checkout more details about the `cleanupPolicy` [here](../CRDs/Cluster/ceph-cluster-crd.md#cleanup-policy)

2. Delete the `CephCluster` CR.

```console
kubectl -n rook-ceph delete cephcluster rook-ceph
```

3. Verify that the cluster CR has been deleted before continuing to the next step.

```console
kubectl -n rook-ceph get cephcluster
```

If the `cleanupPolicy` was applied, wait for the `rook-ceph-cleanup` jobs to be completed on all the nodes.
These jobs will perform the following operations:

* Delete the namespace directory under `dataDirHostPath`, for example `/var/lib/rook/rook-ceph`, on all the nodes
* Wipe the data on the drives on all the nodes where OSDs were running in this cluster

!!! note
    The cleanup jobs might not start if the resources created on top of Rook Cluster are not deleted completely.
    See [deleting block and file artifacts](#delete-the-block-and-file-artifacts)

### Delete the Operator Resources

Remove the Rook operator, RBAC, and CRDs, and the `rook-ceph` namespace.

```console
kubectl delete -f operator.yaml
kubectl delete -f common.yaml
kubectl delete -f crds.yaml
```

### Delete the data on hosts

!!! attention
    The final cleanup step requires deleting files on each host in the cluster. All files under the `dataDirHostPath` property specified in the cluster CRD will need to be deleted. Otherwise, inconsistent state will remain when a new cluster is started.

If the `cleanupPolicy` was not added to the CephCluster CR before deleting the cluster, these manual steps are required to tear down the cluster.

Connect to each machine and delete the namespace directory under `dataDirHostPath`, for example `/var/lib/rook/rook-ceph`.

#### Zapping Devices

Disks on nodes used by Rook for OSDs can be reset to a usable state.
Note that these scripts are not one-size-fits-all. Please use them with discretion to ensure you are
not removing data unrelated to Rook.

A single disk can usually be cleared with some or all of the steps below.

```console
DISK="/dev/sdX"

# Zap the disk to a fresh, usable state (zap-all is important, b/c MBR has to be clean)
sgdisk --zap-all $DISK

# Wipe a large portion of the beginning of the disk to remove more LVM metadata that may be present
dd if=/dev/zero of="$DISK" bs=1M count=100 oflag=direct,dsync

# SSDs may be better cleaned with blkdiscard instead of dd
blkdiscard $DISK

# Inform the OS of partition table changes
partprobe $DISK
```

Ceph can leave LVM and device mapper data that can lock the disks, preventing the disks from being
used again. These steps can help to free up old Ceph disks for reuse. Note that this only needs to
be run once on each node. If you have **only one** Rook cluster and **all** Ceph disks are
being wiped, run the following command.

```console
# This command hangs on some systems: with caution, 'dmsetup remove_all --force' can be used
ls /dev/mapper/ceph-* | xargs -I% -- dmsetup remove %

# ceph-volume setup can leave ceph-<UUID> directories in /dev and /dev/mapper (unnecessary clutter)
rm -rf /dev/ceph-*
rm -rf /dev/mapper/ceph--*
```

If disks are still reported locked, rebooting the node often helps clear LVM-related holds on disks.

If there are multiple Ceph clusters and some disks are not wiped yet, it is necessary to manually
determine which disks map to which device mapper devices.

### Troubleshooting

The most common issue cleaning up the cluster is that the `rook-ceph` namespace or the cluster CRD remain indefinitely in the `terminating` state. A namespace cannot be removed until all of its resources are removed, so determine which resources are pending termination.

If a pod is still terminating, consider forcefully terminating the pod (`kubectl -n rook-ceph delete pod <name>`).

```console
kubectl -n rook-ceph get pod
```

If the cluster CRD still exists even though it has been deleted, see the next section on removing the finalizer.

```console
kubectl -n rook-ceph get cephcluster
```

#### Removing the Cluster CRD Finalizer

When a Cluster CRD is created, a [finalizer](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/#finalizers) is added automatically by the Rook operator. The finalizer will allow the operator to ensure that before the cluster CRD is deleted, all block and file mounts will be cleaned up. Without proper cleanup, pods consuming the storage will be hung indefinitely until a system reboot.

The operator is responsible for removing the finalizer after the mounts have been cleaned up.
If for some reason the operator is not able to remove the finalizer (i.e., the operator is not running anymore), delete the finalizer manually with the following command:

```console
for CRD in $(kubectl get crd -n rook-ceph | awk '/ceph.rook.io/ {print $1}'); do
    kubectl get -n rook-ceph "$CRD" -o name | \
    xargs -I {} kubectl patch -n rook-ceph {} --type merge -p '{"metadata":{"finalizers": []}}'
done
```

If the namespace is still stuck in Terminating state, check which resources are holding up the deletion and remove their finalizers as well:

```console
kubectl api-resources --verbs=list --namespaced -o name \
  | xargs -n 1 kubectl get --show-kind --ignore-not-found -n rook-ceph
```

#### Remove critical resource finalizers

Rook adds a finalizer `ceph.rook.io/disaster-protection` to resources critical to the Ceph cluster so that the resources will not be accidentally deleted.

The operator is responsible for removing the finalizers when a CephCluster is deleted.
If the operator is not able to remove the finalizers (i.e., the operator is not running anymore), remove the finalizers manually:

```console
kubectl -n rook-ceph patch configmap rook-ceph-mon-endpoints --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch secrets rook-ceph-mon --type merge -p '{"metadata":{"finalizers": []}}'
```
