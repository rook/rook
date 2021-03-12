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

## Liveness Sidecar

All CSI pods are deployed with a sidecar container that provides a prometheus metric for tracking if the CSI plugin is alive and running.
These metrics are meant to be collected by prometheus but can be accesses through a GET request to a specific node ip.
for example `curl -X get http://[pod ip]:[liveness-port][liveness-path]  2>/dev/null | grep csi`
the expected output should be

```console
curl -X GET http://10.109.65.142:9080/metrics 2>/dev/null | grep csi
```

>```
># HELP csi_liveness Liveness Probe
># TYPE csi_liveness gauge
>csi_liveness 1
>```

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
