---
title: CSI provisioner and driver
---

!!! attention
    This feature is experimental and will not support upgrades to future versions.

In version 1.9.1, Rook is able to deploy the experimental NFS Ceph CSI driver. This requires Ceph
CSI version 3.6.0 or above. We recommend Ceph v16.2.7 or above.

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

You may need to enable or disable the Ceph orchestrator. Follow the same steps documented
[above](#enable-the-ceph-orchestrator-if-necessary) based on your Ceph version and desires.

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
`share` path which can be used as the export path when [mounting the export manually](nfs.md#mounting-exports).
