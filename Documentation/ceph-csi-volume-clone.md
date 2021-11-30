---
title: Volume clone
weight: 3250
indent: true
---

The CSI Volume Cloning feature adds support for specifying existing PVCs in the
`dataSource` field to indicate a user would like to clone a Volume.

A Clone is defined as a duplicate of an existing Kubernetes Volume that can be
consumed as any standard Volume would be. The only difference is that upon
provisioning, rather than creating a "new" empty Volume, the back end device
creates an exact duplicate of the specified Volume.

Refer to [clone-doc](https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/)
for more info.

## RBD Volume Cloning

### Volume Clone Prerequisites

 1. Requires Kubernetes v1.16+ which supports volume clone.
 2. Ceph-csi diver v3.0.0+ which supports volume clone.

### Volume Cloning

In
[pvc-clone](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples/csi/rbd/pvc-clone.yaml),
`dataSource` should be the name of the `PVC` which is already created by RBD
CSI driver. The `dataSource` kind should be the `PersistentVolumeClaim` and also storageclass
should be same as the source `PVC`.

Create a new PVC Clone from the PVC

```console
kubectl create -f deploy/examples/csi/rbd/pvc-clone.yaml
```

### Verify RBD volume Clone PVC Creation

```console
kubectl get pvc
```

>```
>NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS          AGE
>rbd-pvc           Bound    pvc-74734901-577a-11e9-b34f-525400581048   1Gi        >RWO            rook-ceph-block       34m
>rbd-pvc-clone     Bound    pvc-70473135-577f-11e9-b34f-525400581048   1Gi        RWO            rook-ceph-block       8s
>```

## RBD clone resource Cleanup

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/rbd/pvc-clone.yaml
```

## CephFS Volume Cloning

### Volume Clone Prerequisites

 1. Requires Kubernetes v1.16+ which supports volume clone.
 2. Ceph-csi diver v3.1.0+ which supports volume clone.

### Volume Cloning

In
[pvc-clone](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples/csi/cephfs/pvc-clone.yaml),
`dataSource` should be the name of the `PVC` which is already created by CephFS
CSI driver. The `dataSource` kind should be the `PersistentVolumeClaim` and also storageclass
should be same as the source `PVC`.

Create a new PVC Clone from the PVC

```console
kubectl create -f deploy/examples/csi/cephfs/pvc-clone.yaml
```

### Verify CephFS volume Clone PVC Creation

```console
kubectl get pvc
```

>```
>NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
>cephfs-pvc        Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            rook-cephfs    39m
>cephfs-pvc-clone  Bound    pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWX            rook-cephfs    8s
>```

## CephFS clone resource Cleanup

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/cephfs/pvc-clone.yaml
```
