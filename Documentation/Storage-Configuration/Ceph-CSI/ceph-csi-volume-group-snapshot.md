---
title: Volume Group Snapshots
---

Ceph provides the ability to create crash-consistent snapshots of multiple volumes.
A group snapshot represents “copies” from multiple volumes that are taken at the same point in time.
A group snapshot can be used either to rehydrate new volumes (pre-populated with the snapshot data)
or to restore existing volumes to a previous state (represented by the snapshots)


## Prerequisites

- Install the [snapshot controller, volume group snapshot and snapshot CRDs](https://github.com/kubernetes-csi/external-snapshotter/tree/master#usage),
refer to VolumeGroupSnapshot documentation
[here](https://github.com/kubernetes-csi/external-snapshotter/tree/master#volume-group-snapshot-support) for more details.

- A `VolumeGroupSnapshotClass` is needed for the volume group snapshot to work. The purpose of a `VolumeGroupSnapshotClass` is
defined in [the kubernetes
documentation](https://kubernetes.io/blog/2023/05/08/kubernetes-1-27-volume-group-snapshot-alpha/).
In short, as the documentation describes it:

!!! info
    Created by cluster administrators to describe how volume group snapshots
    should be created. including the driver information, the deletion policy, etc.

## RBD Volume Group Snapshots

### RBD VolumeGroupSnapshotClass

In [VolumeGroupSnapshotClass](https://github.com/rook/rook/tree/master/deploy/examples/csi/rbd/groupsnapshotclass.yaml),
the `csi.storage.k8s.io/group-snapshotter-secret-name` parameter references the
name of the secret created for the rbd-plugin and `pool` to reflect the Ceph pool name.

In the `VolumeGroupSnapshotClass`, update the value of the `clusterID` field to match the namespace
that Rook is running in. When Ceph CSI is deployed by Rook, the operator will automatically
maintain a configmap whose contents will match this key. By default this is
"rook-ceph".

```console
kubectl create -f deploy/examples/csi/rbd/groupsnapshotclass.yaml
```

### RBD VolumeGroupSnapshot

In [VolumeGroupSnapshot](https://github.com/rook/rook/tree/master/deploy/examples/csi/rbd/groupsnapshot.yaml),
`volumeGroupSnapshotClassName` is the name of the `VolumeGroupSnapshotClass`
previously created. The labels inside `matchLabels` must be present on the
PVCs that are already created by the RBD CSI driver.

```console
kubectl create -f deploy/examples/csi/rbd/groupsnapshot.yaml
```

### Verify RBD GroupSnapshot Creation

```console
$ kubectl get volumegroupsnapshotclass
NAME                              DRIVER                          DELETIONPOLICY   AGE
csi-rbdplugin-groupsnapclass      rook-ceph.rbd.csi.ceph.com      Delete           21m
```

```console
$ kubectl get volumegroupsnapshot
NAME                       READYTOUSE   VOLUMEGROUPSNAPSHOTCLASS          VOLUMEGROUPSNAPSHOTCONTENT                              CREATIONTIME   AGE
rbd-groupsnapshot          true         csi-rbdplugin-groupsnapclass      groupsnapcontent-d13f4d95-8822-4729-9586-4f222a3f788e   5m37s          5m39s
```

The snapshot will be ready to restore to a new PVC when `READYTOUSE` field of the
`volumegroupsnapshot` is set to true.

### Restore the RBD volume group snapshot to a new PVC

Find the name of the snapshots created by the `VolumeGroupSnapshot` first by running:

```console
$ kubectl get volumegroupsnapshot/rbd-groupsnapshot -o=jsonpath='{range .status.pvcVolumeSnapshotRefList[*]}PVC: {.persistentVolumeClaimRef.name}, Snapshot: {.volumeSnapshotRef.name}{"\n"}{end}'
PVC: rbd-pvc, Snapshot: snapshot-9d21b143904c10f49ddc92664a7e8fe93c23387d0a88549c14337484ebaf1011-2024-09-12-3.49.13
```

It will list the PVC's name followed by its snapshot name.

In
[pvc-restore](https://github.com/rook/rook/tree/master/deploy/examples/csi/rbd/pvc-restore.yaml),
`dataSource` is one of the `Snapshot` that we just
found. The `dataSource` kind must be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f deploy/examples/csi/rbd/pvc-restore.yaml
```

### Verify RBD Restore PVC Creation

```console
$ kubectl get pvc
rbd-pvc           Bound    pvc-9ae60bf9-4931-4f9a-9de1-7f45f31fe4da   1Gi        RWO            rook-cephfs    <unset>                 171m
rbd-pvc-restore   Bound    pvc-b4b73cbb-5061-48c7-9ac8-e1202508cf97   1Gi        RWO            rook-cephfs    <unset>                 46s
```

### RBD volume group snapshot resource Cleanup

To clean the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/rbd/pvc-restore.yaml
kubectl delete -f deploy/examples/csi/rbd/groupsnapshot.yaml
kubectl delete -f deploy/examples/csi/rbd/groupsnapshotclass.yaml
```

## CephFS Volume Group Snapshots

### CephFS VolumeGroupSnapshotClass

In [VolumeGroupSnapshotClass](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/groupsnapshotclass.yaml),
the `csi.storage.k8s.io/group-snapshotter-secret-name` parameter references the
name of the secret created for the cephfs-plugin.

In the `VolumeGroupSnapshotClass`, update the value of the `clusterID` field to match the namespace
that Rook is running in. When Ceph CSI is deployed by Rook, the operator will automatically
maintain a configmap whose contents will match this key. By default this is
"rook-ceph".

```console
kubectl create -f deploy/examples/csi/cephfs/groupsnapshotclass.yaml
```

### CephFS VolumeGroupSnapshot

In [VolumeGroupSnapshot](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/groupsnapshot.yaml),
`volumeGroupSnapshotClassName` is the name of the `VolumeGroupSnapshotClass`
previously created. The labels inside `matchLabels` must be present on the
PVCs that are already created by the CephFS CSI driver.

```console
kubectl create -f deploy/examples/csi/cephfs/groupsnapshot.yaml
```

### Verify CephFS GroupSnapshot Creation

```console
$ kubectl get volumegroupsnapshotclass
NAME                              DRIVER                          DELETIONPOLICY   AGE
csi-cephfsplugin-groupsnapclass   rook-ceph.cephfs.csi.ceph.com   Delete           21m
```

```console
$ kubectl get volumegroupsnapshot
NAME                       READYTOUSE   VOLUMEGROUPSNAPSHOTCLASS          VOLUMEGROUPSNAPSHOTCONTENT                              CREATIONTIME   AGE
cephfs-groupsnapshot       true         csi-cephfsplugin-groupsnapclass   groupsnapcontent-d13f4d95-8822-4729-9586-4f222a3f788e   5m37s          5m39s
```

The snapshot will be ready to restore to a new PVC when `READYTOUSE` field of the
`volumegroupsnapshot` is set to true.

### Restore the CephFS volume group snapshot to a new PVC

Find the name of the snapshots created by the `VolumeGroupSnapshot` first by running:

```console
$ kubectl get volumegroupsnapshot/cephfs-groupsnapshot -o=jsonpath='{range .status.pvcVolumeSnapshotRefList[*]}PVC: {.persistentVolumeClaimRef.name}, Snapshot: {.volumeSnapshotRef.name}{"\n"}{end}'
PVC: cephfs-pvc, Snapshot: snapshot-9d21b143904c10f49ddc92664a7e8fe93c23387d0a88549c14337484ebaf1011-2024-09-12-3.49.13
```

It will list the PVC's name followed by its snapshot name.

In
[pvc-restore](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/pvc-restore.yaml),
`dataSource` is one of the `Snapshot` that we just
found. The `dataSource` kind must be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f deploy/examples/csi/cephfs/pvc-restore.yaml
```

### Verify CephFS Restore PVC Creation

```console
$ kubectl get pvc
cephfs-pvc           Bound    pvc-9ae60bf9-4931-4f9a-9de1-7f45f31fe4da   1Gi        RWO            rook-cephfs    <unset>                 171m
cephfs-pvc-restore   Bound    pvc-b4b73cbb-5061-48c7-9ac8-e1202508cf97   1Gi        RWO            rook-cephfs    <unset>                 46s
```

### CephFS volume group snapshot resource Cleanup

To clean the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/cephfs/pvc-restore.yaml
kubectl delete -f deploy/examples/csi/cephfs/groupsnapshot.yaml
kubectl delete -f deploy/examples/csi/cephfs/groupsnapshotclass.yaml
```
