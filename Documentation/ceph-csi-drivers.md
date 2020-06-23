---
title: Ceph CSI
weight: 3200
indent: true
---

# Ceph CSI Drivers

There are two CSI drivers integrated with Rook that will enable different scenarios:

* RBD: This driver is optimized for RWO pod access where only one pod may access the storage
* CephFS: This driver allows for RWX with one or more pods accessing the same storage

The drivers are enabled automatically with the Rook operator. They will be started
in the same namespace as the operator when the first CephCluster CR is created.

For documentation on consuming the storage:

* RBD: See the [Block Storage](ceph-block.md) topic
* CephFS: See the [Shared Filesystem](ceph-filesystem.md) topic

## Configure CSI Drivers in non-default namespace

If you've deployed the Rook operator in a namespace other than "rook-ceph",
change the prefix in the provisioner to match the namespace you used. For
example, if the Rook operator is running in the namespace "my-namespace" the
provisioner value should be "my-namespace.rbd.csi.ceph.com". The same provisioner
name needs to be set in both the storageclass and snapshotclass.

## RBD Snapshots

### Prerequisites

1. Requires kubernetes v1.17+ which supports snapshot beta.

2. Install the new snapshot controller and snapshot beta CRD. More info can be found
[here](https://github.com/kubernetes-csi/external-snapshotter/tree/master#usage)

### SnapshotClass

You need to create the `SnapshotClass`. The purpose of a `SnapshotClass` is
defined in [the kubernetes
documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/).
In short, as the documentation describes it:

> Just like StorageClass provides a way for administrators to describe the
> “classes” of storage they offer when provisioning a volume,
> VolumeSnapshotClass provides a way to describe the “classes” of storage when
> provisioning a volume snapshot.

In [snapshotClass](/cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml),
the `csi.storage.k8s.io/snapshotter-secret-name` parameter should reference the
name of the secret created for the rbdplugin and `pool` to reflect the Ceph pool name.

Update the value of the `clusterID` field to match the namespace that rook is
running in. When Ceph CSI is deployed by Rook, the operator will automatically
maintain a config map whose contents will match this key. By default this is
"rook-ceph".

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml
```

### Volumesnapshot

In [snapshot](/cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml),
`volumeSnapshotClassName` should be the name of the `VolumeSnapshotClass`
previously created. The `persistentVolumeClaimName` should be the name of the
PVC you created earlier.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml
```

### Verify RBD Snapshot Creation

```console
$ kubectl get volumesnapshotclass
NAME                      DRIVER             DELETIONPOLICY   AGE
csi-rbdplugin-snapclass   rbd.csi.ceph.com   Delete           3h55m

$ kubectl get volumesnapshot
NAME               READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS             SNAPSHOTCONTENT                                    CREATIONTIME   AGE
rbd-pvc-snapshot   true         rbd-pvc                             1Gi           csi-rbdplugin-snapclass   snapcontent-79090db0-7c66-4b18-bf4a-634772c7cac7   3h50m          3h51m
```

The snapshot will be ready to restore to a new PVC when `READYTOUSE` field of the
volumesnapshot is set to true

### Restore the snapshot to a new PVC

In
[pvc-restore](/cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml),
`dataSource` should be the name of the `VolumeSnapshot` previously
created. The kind should be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml
```

### Verify RBD Clone PVC Creation

```bash
$ kubectl get pvc
NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rbd-pvc           Bound    pvc-84294e34-577a-11e9-b34f-525400581048   1Gi        RWO            csi-rbd        34m
rbd-pvc-restore   Bound    pvc-575537bf-577f-11e9-b34f-525400581048   1Gi        RWO            csi-rbd        8s
```

## RBD resource Cleanup

To clean your cluster of the resources created by this example, run the following:

if you have tested snapshot, delete snapshotclass, snapshot and pvc-restore
created to test snapshot feature

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/pvc-restore.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/rbd/snapshotclass.yaml
```

## Liveness Sidecar

All CSI pods are deployed with a sidecar container that provides a prometheus metric for tracking if the CSI plugin is alive and runnning.
These metrics are meant to be collected by prometheus but can be acceses through a GET request to a specific node ip.
for example `curl -X get http://[pod ip]:[liveness-port][liveness-path]  2>/dev/null | grep csi`
the expected output should be

```console
$ curl -X GET http://10.109.65.142:9080/metrics 2>/dev/null | grep csi
# HELP csi_liveness Liveness Probe
# TYPE csi_liveness gauge
csi_liveness 1
```

Check the [monitoring doc](ceph-monitoring.md) to see how to integrate CSI
liveness and grpc metrics into ceph monitoring.

## Dynamically Expand Volume

### Prerequisite

* For filesystem resize to be supported for your Kubernetes cluster, the
  kubernetes version running in your cluster should be >= v1.15 and for block
  volume resize support the Kubernetes version should be >= v1.16. Also,
  `ExpandCSIVolumes` feature gate has to be enabled for the volume resize
  functionality to work.

To expand the PVC the controlling StorageClass must have `allowVolumeExpansion`
set to `true`. `csi.storage.k8s.io/controller-expand-secret-name` and
`csi.storage.k8s.io/controller-expand-secret-namespace` values set in
storageclass. Now expand the PVC by editing the PVC
`pvc.spec.resource.requests.storage` to a higher values than the current size.
Once PVC is expanded on backend and same is reflected size is reflected on
application mountpoint, the status capacity `pvc.status.capacity.storage` of
PVC will be updated to new size.
