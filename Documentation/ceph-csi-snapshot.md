---
title: Snapshots
weight: 3250
indent: true
---

## Prerequisites

* Requires Kubernetes v1.17+ which supports snapshot beta.

* Install the new snapshot controller and snapshot beta CRD. More info can be found
[here](https://github.com/kubernetes-csi/external-snapshotter/tree/master#usage)

Note: If the Kubernetes distributor you are using does not supports the snapshot beta,
still you can use the Alpha snapshots. refer to
[snapshot](https://github.com/rook/rook/blob/release-1.3/Documentation/ceph-csi-drivers.md#rbd-snapshots)
on how to use snapshots.

* We also need a `VolumeSnapshotClass` for volume snapshot to work. The purpose of a `VolumeSnapshotClass` is
defined in [the kubernetes
documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/).
In short, as the documentation describes it:

> Just like StorageClass provides a way for administrators to describe the
> “classes” of storage they offer when provisioning a volume,
> VolumeSnapshotClass provides a way to describe the “classes” of storage when
> provisioning a volume snapshot.


## RBD Snapshots

### VolumeSnapshotClass

In [VolumeSnapshotClass](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml),
the `csi.storage.k8s.io/snapshotter-secret-name` parameter should reference the
name of the secret created for the rbdplugin and `pool` to reflect the Ceph pool name.

Update the value of the `clusterID` field to match the namespace that Rook is
running in. When Ceph CSI is deployed by Rook, the operator will automatically
maintain a configmap whose contents will match this key. By default this is
"rook-ceph".

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml
```

### Volumesnapshot

In [snapshot](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml),
`volumeSnapshotClassName` should be the name of the `VolumeSnapshotClass`
previously created. The `persistentVolumeClaimName` should be the name of the
PVC which is already created by the RBD CSI driver.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml
```

### Verify RBD Snapshot Creation

```console
kubectl get volumesnapshotclass
```

>```
>NAME                      DRIVER                       DELETIONPOLICY   AGE
>csi-rbdplugin-snapclass   rook-ceph.rbd.csi.ceph.com   Delete           3h55m
>```

```console
kubectl get volumesnapshot
```

>```
>NAME               READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS             SNAPSHOTCONTENT                                    CREATIONTIME   AGE
>rbd-pvc-snapshot   true         rbd-pvc                             1Gi           csi-rbdplugin-snapclass   snapcontent-79090db0-7c66-4b18-bf4a-634772c7cac7   3h50m          3h51m
>```

The snapshot will be ready to restore to a new PVC when the `READYTOUSE` field of the
`volumesnapshot` is set to true.

### Restore the snapshot to a new PVC

In
[pvc-restore](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml),
`dataSource` should be the name of the `VolumeSnapshot` previously
created. The `dataSource` kind should be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml
```

### Verify RBD Clone PVC Creation

```console
kubectl get pvc
```
>```
>NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS          AGE
>rbd-pvc           Bound    pvc-84294e34-577a-11e9-b34f-525400581048   1Gi        RWO            rook-ceph-block       34m
>rbd-pvc-restore   Bound    pvc-575537bf-577f-11e9-b34f-525400581048   1Gi        RWO            rook-ceph-block       8s
>```

## RBD snapshot resource Cleanup

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml
```

## CephFS Snapshots

### VolumeSnapshotClass

In [VolumeSnapshotClass](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/cephfs/snapshotclass.yaml),
the `csi.storage.k8s.io/snapshotter-secret-name` parameter should reference the
name of the secret created for the cephfsplugin.

In the volumesnapshotclass, update the value of the `clusterID` field to match the namespace that Rook is
running in. When Ceph CSI is deployed by Rook, the operator will automatically
maintain a configmap whose contents will match this key. By default this is
"rook-ceph".


```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/cephfs/snapshotclass.yaml
```

### Volumesnapshot

In [snapshot](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/cephfs/snapshot.yaml),
`volumeSnapshotClassName` should be the name of the `VolumeSnapshotClass`
previously created. The `persistentVolumeClaimName` should be the name of the
PVC which is already created by the CephFS CSI driver.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/cephfs/snapshot.yaml
```

### Verify CephFS Snapshot Creation

```console
kubectl get volumesnapshotclass
```
>```
>NAME                        DRIVER                          DELETIONPOLICY   AGE
>csi-cephfslugin-snapclass   rook-ceph.cephfs.csi.ceph.com   Delete           3h55m
>```
```console
kubectl get volumesnapshot
```

>```
>NAME                  READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT  RESTORESIZE   SNAPSHOTCLASS                SNAPSHOTCONTENT                                   CREATIONTIME   AGE
>cephfs-pvc-snapshot   true         cephfs-pvc                         1Gi           csi-cephfsplugin-snapclass   snapcontent-34476204-a14a-4d59-bfbc-2bbba695652c  3h50m          3h51m
>```

The snapshot will be ready to restore to a new PVC when `READYTOUSE` field of the
`volumesnapshot` is set to true.

### Restore the snapshot to a new PVC

In
[pvc-restore](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/cephfs/pvc-restore.yaml),
`dataSource` should be the name of the `VolumeSnapshot` previously
created. The `dataSource` kind should be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/cephfs/pvc-restore.yaml
```

### Verify CephFS Restore PVC Creation

```console
kubectl get pvc
```

>```
>NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
>cephfs-pvc           Bound    pvc-74734901-577a-11e9-b34f-525400581048   1Gi        RWX            rook-cephfs       55m
>cephfs-pvc-restore   Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            rook-cephfs       34s
>```

## CephFS snapshot resource Cleanup

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/cephfs/pvc-restore.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/cephfs/snapshot.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/cephfs/snapshotclass.yaml
```

## Limitations

* There is a limit of 400 snapshots per cephFS filesystem.
* The PVC cannot be deleted if it has snapshots. make sure all the snapshots on the PVC are deleted before you delete the PVC.
