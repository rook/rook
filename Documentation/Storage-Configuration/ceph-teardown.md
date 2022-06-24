---
title: Cleanup
---

## Cleaning up a Cluster

If you want to tear down the cluster and bring up a new one, be aware of the following resources that will need to be cleaned up:

* `rook-ceph` namespace: The Rook operator and cluster created by `operator.yaml` and `cluster.yaml` (the cluster CRD)
* `/var/lib/rook`: Path on each host in the cluster where configuration is cached by the ceph mons and osds

Note that if you changed the default namespaces or paths such as `dataDirHostPath` in the sample yaml files, you will need to adjust these namespaces and paths throughout these instructions.

If you see issues tearing down the cluster, see the [Troubleshooting](#troubleshooting) section below.

If you are tearing down a cluster frequently for development purposes, it is instead recommended to use an environment such as Minikube that can easily be reset without worrying about any of these steps.

### Delete the Block and File artifacts

First you will need to clean up the resources created on top of the Rook cluster.

These commands will clean up the resources from the [block](Block-Storage-RBD/block-storage.md#teardown) and [file](Shared-Filesystem-CephFS/filesystem-storage.md#teardown) walkthroughs (unmount volumes, delete volume claims, etc). If you did not complete those parts of the walkthrough, you can skip these instructions:

```console
kubectl delete -f ../wordpress.yaml
kubectl delete -f ../mysql.yaml
kubectl delete -n rook-ceph cephblockpool replicapool
kubectl delete storageclass rook-ceph-block
kubectl delete -f csi/cephfs/kube-registry.yaml
kubectl delete storageclass csi-cephfs
```

After those block and file resources have been cleaned up, you can then delete your Rook cluster. This is important to delete **before removing the Rook operator and agent or else resources may not be cleaned up properly**.

### Delete the CephCluster CRD

Edit the `CephCluster` and add the `cleanupPolicy`

WARNING: DATA WILL BE PERMANENTLY DELETED AFTER DELETING THE `CephCluster` CR WITH `cleanupPolicy`.

```console
kubectl -n rook-ceph patch cephcluster rook-ceph --type merge -p '{"spec":{"cleanupPolicy":{"confirmation":"yes-really-destroy-data"}}}'
```

Once the cleanup policy is enabled, any new configuration changes in the CephCluster will be blocked. Nothing will happen until the deletion of the CR is requested, so this `cleanupPolicy` change can still be reverted if needed.

Checkout more details about the `cleanupPolicy` [here](../CRDs/Cluster/ceph-cluster-crd.md#cleanup-policy)

Delete the `CephCluster` CR.

```console
kubectl -n rook-ceph delete cephcluster rook-ceph
```

Verify that the cluster CR has been deleted before continuing to the next step.

```console
kubectl -n rook-ceph get cephcluster
```

If the `cleanupPolicy` was applied, then wait for the `rook-ceph-cleanup` jobs to be completed on all the nodes.
These jobs will perform the following operations:

* Delete the directory `/var/lib/rook` (or the path specified by the `dataDirHostPath`) on all the nodes
* Wipe the data on the drives on all the nodes where OSDs were running in this cluster

Note: The cleanup jobs might not start if the resources created on top of Rook Cluster are not deleted completely. [See](#delete-the-block-and-file-artifacts)

### Delete the Operator and related Resources

This will begin the process of the Rook Ceph operator and all other resources being cleaned up.
This includes related resources such as the agent and discover daemonsets with the following commands:

```console
kubectl delete -f operator.yaml
kubectl delete -f common.yaml
kubectl delete -f crds.yaml
```

If the `cleanupPolicy` was applied and the cleanup jobs have completed on all the nodes, then the cluster tear down has been successful. If you skipped adding the `cleanupPolicy` then follow the manual steps mentioned below to tear down the cluster.

### Delete the data on hosts

!!! attention
    The final cleanup step requires deleting files on each host in the cluster. All files under the `dataDirHostPath` property specified in the cluster CRD will need to be deleted. Otherwise, inconsistent state will remain when a new cluster is started.

Connect to each machine and delete `/var/lib/rook`, or the path specified by the `dataDirHostPath`.

In the future this step will not be necessary when we build on the K8s local storage feature.

If you modified the demo settings, additional cleanup is up to you for devices, host paths, etc.

#### Zapping Devices

Disks on nodes used by Rook for osds can be reset to a usable state with methods suggested below.
Note that these scripts are not one-size-fits-all. Please use them with discretion to ensure you are
not removing data unrelated to Rook and/or Ceph.

Disks can be zapped fairly easily. A single disk can usually be cleared with some or all of the
steps below.

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
used again. These steps can help to free up old Ceph disks for re-use. Note that this only needs to
be run once on each node and assumes that **all** Ceph disks are being wiped. If only some disks are
being wiped, you will have to manually determine which disks map to which device mapper devices.

```console
# This command hangs on some systems: with caution, 'dmsetup remove_all --force' can be used
ls /dev/mapper/ceph-* | xargs -I% -- dmsetup remove %

# ceph-volume setup can leave ceph-<UUID> directories in /dev and /dev/mapper (unnecessary clutter)
rm -rf /dev/ceph-*
rm -rf /dev/mapper/ceph--*
```

If disks are still reported locked, rebooting the node often helps clear LVM-related holds on disks.

### Troubleshooting

If the cleanup instructions are not executed in the order above, or you otherwise have difficulty cleaning up the cluster, here are a few things to try.

The most common issue cleaning up the cluster is that the `rook-ceph` namespace or the cluster CRD remain indefinitely in the `terminating` state. A namespace cannot be removed until all of its resources are removed, so look at which resources are pending termination.

Look at the pods:

```console
kubectl -n rook-ceph get pod
```

If a pod is still terminating, you will need to wait or else attempt to forcefully terminate it (`kubectl delete pod <name>`).

Now look at the cluster CRD:

```console
kubectl -n rook-ceph get cephcluster
```

If the cluster CRD still exists even though you have executed the delete command earlier, see the next section on removing the finalizer.

#### Removing the Cluster CRD Finalizer

When a Cluster CRD is created, a [finalizer](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/#finalizers) is added automatically by the Rook operator. The finalizer will allow the operator to ensure that before the cluster CRD is deleted, all block and file mounts will be cleaned up. Without proper cleanup, pods consuming the storage will be hung indefinitely until a system reboot.

The operator is responsible for removing the finalizer after the mounts have been cleaned up.
If for some reason the operator is not able to remove the finalizer (i.e., the operator is not running anymore), you can delete the finalizer manually with the following command:

```console
for CRD in $(kubectl get crd -n rook-ceph | awk '/ceph.rook.io/ {print $1}'); do
    kubectl get -n rook-ceph "$CRD" -o name | \
    xargs -I {} kubectl patch -n rook-ceph {} --type merge -p '{"metadata":{"finalizers": [null]}}'
done
```

This command will patch the following CRDs on v1.3:

```console
cephblockpools.ceph.rook.io
cephclients.ceph.rook.io
cephfilesystems.ceph.rook.io
cephnfses.ceph.rook.io
cephobjectstores.ceph.rook.io
cephobjectstoreusers.ceph.rook.io
```

Within a few seconds you should see that the cluster CRD has been deleted and will no longer block other cleanup such as deleting the `rook-ceph` namespace.

If the namespace is still stuck in Terminating state, you can check which resources are holding up the deletion and remove the finalizers and delete those

```console
kubectl api-resources --verbs=list --namespaced -o name \
  | xargs -n 1 kubectl get --show-kind --ignore-not-found -n rook-ceph
```

#### Remove critical resource finalizers

Rook adds a finalizer `ceph.rook.io/disaster-protection` to resources critical to the Ceph cluster so that the resources will not be accidentally deleted.

The operator is responsible for removing the finalizers when a CephCluster is deleted.
If for some reason the operator is not able to remove the finalizers (i.e., the operator is not running anymore), you can remove the finalizers manually with the following commands:

```console
kubectl -n rook-ceph patch configmap rook-ceph-mon-endpoints --type merge -p '{"metadata":{"finalizers": [null]}}'
kubectl -n rook-ceph patch secrets rook-ceph-mon --type merge -p '{"metadata":{"finalizers": [null]}}'
```
