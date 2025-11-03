---
title: CSI provisioner and driver
---

!!! attention
    This feature is experimental and will not support upgrades to future versions.

For this section, we will refer to Rook's deployment examples in the
[deploy/examples](https://github.com/rook/rook/tree/master/deploy/examples) directory.

## Enabling the CSI drivers

The Ceph CSI NFS provisioner and driver require additional RBAC to operate. Apply the
`deploy/examples/csi/nfs/rbac.yaml` manifest to deploy the additional resources.

Rook will only deploy the Ceph CSI NFS provisioner and driver components when the
`ROOK_CSI_ENABLE_NFS` config is set to `"true"` in the `rook-ceph-operator-config` configmap. Change
the value in your manifest, or patch the resource as below.

```console
kubectl --namespace rook-ceph patch configmap rook-ceph-operator-config --type merge --patch '{"data":{"ROOK_CSI_ENABLE_NFS": "true"}}'
```

!!! note
    The rook-ceph operator Helm chart will deploy the required RBAC and enable the driver
    components if `csi.nfs.enabled` is set to `true`.

## Creating NFS exports via PVC

### Prerequisites

In order to create NFS exports via the CSI driver, you must first create a CephFilesystem to serve
as the underlying storage for the exports, and you must create a CephNFS to run an NFS server that
will expose the exports. RGWs cannot be used for the CSI driver.

From the examples, `filesystem.yaml` creates a CephFilesystem called `myfs`, and `nfs.yaml` creates
an NFS server called `my-nfs`.

You may need to enable or disable the Ceph orchestrator.

You must also create a storage class. Ceph CSI is designed to support any arbitrary Ceph cluster,
but we are focused here only on Ceph clusters deployed by Rook. Let's take a look at a portion of
the example storage class found at `deploy/examples/csi/nfs/storageclass.yaml` and break down how
the values are determined.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-nfs
provisioner: rook-ceph.nfs.csi.ceph.com # [1]
parameters:
  nfsCluster: my-nfs # [2]
  server: rook-ceph-nfs-my-nfs-a # [3]
  clusterID: rook-ceph # [4]
  fsName: myfs # [5]
  pool: myfs-replicated # [6]

  # [7] (entire csi.storage.k8s.io/* section immediately below)
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph

# ... some fields omitted ...
```

1. `provisioner`: **rook-ceph**.nfs.csi.ceph.com because **rook-ceph** is the namespace where the
    CephCluster is installed
2. `nfsCluster`: **my-nfs** because this is the name of the CephNFS
3. `server`: rook-ceph-nfs-**my-nfs**-a because Rook creates this Kubernetes Service for the CephNFS
    named **my-nfs**
4. `clusterID`: **rook-ceph** because this is the namespace where the CephCluster is installed
5. `fsName`: **myfs** because this is the name of the CephFilesystem used to back the NFS exports
6. `pool`: **myfs**-**replicated** because **myfs** is the name of the CephFilesystem defined in
    `fsName` and because **replicated** is the name of a data pool defined in the CephFilesystem
7. `csi.storage.k8s.io/*`: note that these values are shared with the Ceph CSI CephFS provisioner

### Creating a PVC

See `deploy/examples/csi/nfs/pvc.yaml` for an example of how to create a PVC that will create an NFS
export. The export will be created and a PV created for the PVC immediately, even without a Pod to
mount the PVC.

### Attaching an export to a pod

See `deploy/examples/csi/nfs/pod.yaml` for an example of how a PVC can be connected to an
application pod.

### Connecting to an export directly

After a PVC is created successfully, the `share` parameter set on the resulting PV contains the
`share` path which can be used as the export path when
[mounting the export manually](nfs.md#mounting-exports). In the example below
`/0001-0009-rook-ceph-0000000000000001-55c910f9-a1af-11ed-9772-1a471870b2f5` is the export path.

```console
$ kubectl get pv pvc-b559f225-de79-451b-a327-3dbec1f95a1c -o jsonpath='{.spec.csi.volumeAttributes}'
/0001-0009-rook-ceph-0000000000000001-55c910f9-a1af-11ed-9772-1a471870b2f5
```

## Taking snapshots of NFS exports

NFS export PVCs can be snapshotted and later restored to new PVCs.

### Creating snapshots

First, create a VolumeSnapshotClass as in the example [here](https://github.com/rook/rook/tree/master/deploy/examples/csi/nfs/snapshotclass.yaml). The `csi.storage.k8s.io/snapshotter-secret-name` parameter should reference the name of the secret created for the cephfsplugin [here](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/snapshotclass.yaml).

```console
kubectl create -f deploy/examples/csi/nfs/snapshotclass.yaml
```

In [snapshot](https://github.com/rook/rook/tree/master/deploy/examples/csi/nfs/snapshot.yaml),
`volumeSnapshotClassName` should be the name of the VolumeSnapshotClass
previously created. The `persistentVolumeClaimName` should be the name of the
PVC which is already created by the NFS CSI driver.

```console
kubectl create -f deploy/examples/csi/nfs/snapshot.yaml
```

### Verifying snapshots

```console
$ kubectl get volumesnapshotclass
NAME                        DRIVER                          DELETIONPOLICY   AGE
csi-nfslugin-snapclass      rook-ceph.nfs.csi.ceph.com      Delete           3h55m
```

```console
$ kubectl get volumesnapshot
NAME                  READYTOUSE   SOURCEPVC   SOURCESNAPSHOTCONTENT  RESTORESIZE   SNAPSHOTCLASS                SNAPSHOTCONTENT                                   CREATIONTIME   AGE
nfs-pvc-snapshot      true         nfs-pvc                            1Gi           csi-nfsplugin-snapclass      snapcontent-34476204-a14a-4d59-bfbc-2bbba695652c  3h50m          3h51m
```

The snapshot will be ready to restore to a new PVC when `READYTOUSE` field of the
volumesnapshot is set to true.

### Restoring snapshot to a new PVC

In
[pvc-restore](https://github.com/rook/rook/tree/master/deploy/examples/csi/nfs/pvc-restore.yaml),
`dataSource` name should be the name of the VolumeSnapshot previously
created. The `dataSource` kind should be "VolumeSnapshot".

Create a new PVC from the snapshot.

```console
kubectl create -f deploy/examples/csi/nfs/pvc-restore.yaml
```

### Verifying restored PVC Creation

```console
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
nfs-pvc              Bound    pvc-74734901-577a-11e9-b34f-525400581048   1Gi        RWX            rook-nfs          55m
nfs-pvc-restore      Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            rook-nfs          34s
```

### Cleaning up snapshot resource

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/nfs/pvc-restore.yaml
kubectl delete -f deploy/examples/csi/nfs/snapshot.yaml
kubectl delete -f deploy/examples/csi/nfs/snapshotclass.yaml
```

## Cloning NFS exports

### Creating clones

In
[pvc-clone](https://github.com/rook/rook/tree/master/deploy/examples/csi/nfs/pvc-clone.yaml),
`dataSource` should be the name of the PVC which is already created by NFS
CSI driver. The `dataSource` kind should be "PersistentVolumeClaim" and also storageclass
should be same as the source PVC.

Create a new PVC Clone from the PVC as in the example [here](https://github.com/rook/rook/tree/master/deploy/examples/csi/nfs/pvc-clone.yaml).

```console
kubectl create -f deploy/examples/csi/nfs/pvc-clone.yaml
```

### Verifying a cloned PVC

```console
kubectl get pvc
```

```console
NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
nfs-pvc           Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            rook-nfs       39m
nfs-pvc-clone     Bound    pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWX            rook-nfs       8s
```

### Cleaning up clone resources

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f deploy/examples/csi/nfs/pvc-clone.yaml
```

## Consuming NFS from an external source

For consuming NFS services and exports external to the Kubernetes cluster (including those backed by an external standalone Ceph cluster), Rook recommends using Kubernetes regular NFS consumption model. This requires the Ceph admin to create the needed export, while reducing the privileges needed in the client cluster for the NFS volume.

Export and get the nfs client to a particular cephFS filesystem:

```yaml
ceph nfs export create cephfs <nfs-client-name> /test <filesystem-name>
ceph nfs export get <service> <export-name>
```

Create the [PV and PVC](https://github.com/kubernetes-csi/csi-driver-nfs/tree/master/deploy/example) using `nfs-client-server-ip`. It will mount NFS volumes with PersistentVolumes and then mount the PVCs in the [user Pod Application](https://kubernetes.io/docs/concepts/storage/volumes/#nfs) to utilize the NFS type storage.
