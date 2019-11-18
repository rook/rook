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

The remainder of this topic is on the snapshotting feature of the RBD driver.

## RBD Snapshots

Since this feature is still in [alpha
stage](https://kubernetes.io/blog/2018/10/09/introducing-volume-snapshot-alpha-for-kubernetes/)
(k8s 1.12+), make sure to enable the `VolumeSnapshotDataSource` feature gate on
your Kubernetes cluster API server.

```console
--feature-gates=VolumeSnapshotDataSource=true
```

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
`snapshotClassName` should be the name of the `VolumeSnapshotClass` previously
created. The source name should be the name of the PVC you created earlier.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/snapshot.yaml
```

### Verify RBD Snapshot Creation

```console
$ kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
$ kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```

In the toolbox pod, run `rbd snap ls [name-of-your-pvc]`.
The output should be similar to this:

```console
$ rbd snap ls pvc-c20495c0d5de11e8
SNAPID NAME                                                                      SIZE TIMESTAMP
     4 csi-rbd-pvc-c20495c0d5de11e8-snap-4c0b455b-d5fe-11e8-bebb-525400123456 1024 MB Mon Oct 22 13:28:03 2018
```

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

```yaml
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
