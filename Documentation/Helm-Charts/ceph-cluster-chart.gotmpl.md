---
title: Ceph Cluster Helm Chart
---
{{ template "generatedDocsWarning" . }}

Creates Rook resources to configure a [Ceph](https://ceph.io/) cluster using the [Helm](https://helm.sh) package manager.
This chart is a simple packaging of templates that will optionally create Rook resources such as:

* CephCluster, CephFilesystem, and CephObjectStore CRs
* Storage classes to expose Ceph RBD volumes, CephFS volumes, and RGW buckets
* Ingress for external access to the dashboard
* Toolbox

## Prerequisites

* Helm 3.x
* Install the [Rook Operator chart](operator-chart.md)

## Installing

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration.
The [configuration](#configuration) section lists the parameters that can be configured during installation. It is
recommended that the rook operator be installed into the `rook-ceph` namespace. The clusters can be installed
into the same namespace as the operator or a separate namespace.

**Before installing, review the values.yaml to confirm if the default settings need to be updated.**

* If the operator was installed in a namespace other than `rook-ceph`, the namespace
  must be set in the `operatorNamespace` variable.
* Set the desired settings in the `cephClusterSpec`. The [defaults](https://github.com/rook/rook/tree/master/deploy/charts/rook-ceph-cluster/values.yaml)
  are only an example and not likely to apply to your cluster.
* The `monitoring` section should be removed from the `cephClusterSpec`, as it is specified separately in the helm settings.
* The default values for `cephBlockPools`, `cephFileSystems`, and `CephObjectStores` will create one of each, and their corresponding storage classes.
* All Ceph components now have default values for the pod resources. The resources may need to be adjusted in production clusters depending on the load. The resources can also be disabled if Ceph should not be limited (e.g. test clusters).

### **Release**

The `release` channel is the most recent release of Rook that is considered stable for the community.

The example install assumes you have first installed the [Rook Operator Helm Chart](operator-chart.md)
and created your customized values.yaml.

```console
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster \
   --set operatorNamespace=rook-ceph rook-release/rook-ceph-cluster -f values.yaml
```

!!! Note
    --namespace specifies the cephcluster namespace, which may be different from the rook operator namespace.

## Configuration

The following table lists the configurable parameters of the rook-operator chart and their default values.

{{ template "chart.valuesTable" . }}

### **Ceph Cluster Spec**

The `CephCluster` CRD takes its spec from `cephClusterSpec.*`. This is not an exhaustive list of parameters.
For the full list, see the [Cluster CRD](../CRDs/Cluster/ceph-cluster-crd.md) topic.

The cluster spec example is for a converged cluster where all the Ceph daemons are running locally,
as in the host-based example (cluster.yaml). For a different configuration such as a
PVC-based cluster (cluster-on-pvc.yaml), external cluster (cluster-external.yaml),
or stretch cluster (cluster-stretched.yaml), replace this entire `cephClusterSpec`
with the specs from those examples.

### **Ceph Block Pools**

The `cephBlockPools` array in the values file will define a list of CephBlockPool as described in the table below.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `name` | The name of the CephBlockPool | `ceph-blockpool` |
| `spec` | The CephBlockPool spec, see the [CephBlockPool](../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) documentation. | `{}` |
| `storageClass.enabled` | Whether a storage class is deployed alongside the CephBlockPool | `true` |
| `storageClass.isDefault` | Whether the storage class will be the default storage class for PVCs. See [PersistentVolumeClaim documentation](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) for details. | `true` |
| `storageClass.name` | The name of the storage class | `ceph-block` |
| `storageClass.annotations` | Additional storage class annotations | `{}` |
| `storageClass.labels` | Additional storage class labels | `{}` |
| `storageClass.parameters` | See [Block Storage](../Storage-Configuration/Block-Storage-RBD/block-storage.md) documentation or the helm values.yaml for suitable values | see values.yaml |
| `storageClass.reclaimPolicy` | The default [Reclaim Policy](https://kubernetes.io/docs/concepts/storage/storage-classes/#reclaim-policy) to apply to PVCs created with this storage class. | `Delete` |
| `storageClass.allowVolumeExpansion` | Whether [volume expansion](https://kubernetes.io/docs/concepts/storage/storage-classes/#allow-volume-expansion) is allowed by default. | `true` |
| `storageClass.mountOptions` | Specifies the mount options for storageClass | `[]` |
| `storageClass.allowedTopologies` | Specifies the [allowedTopologies](https://kubernetes.io/docs/concepts/storage/storage-classes/#allowed-topologies) for storageClass | `[]` |

### **Ceph File Systems**

The `cephFileSystems` array in the values file will define a list of CephFileSystem as described in the table below.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `name` | The name of the CephFileSystem | `ceph-filesystem` |
| `spec` | The CephFileSystem spec, see the [CephFilesystem CRD](../CRDs/Shared-Filesystem/ceph-filesystem-crd.md) documentation. | see values.yaml |
| `storageClass.enabled` | Whether a storage class is deployed alongside the CephFileSystem | `true` |
| `storageClass.name` | The name of the storage class | `ceph-filesystem` |
| `storageClass.annotations` | Additional storage class annotations | `{}` |
| `storageClass.labels` | Additional storage class labels | `{}` |
| `storageClass.pool` | The name of [Data Pool](../CRDs/Shared-Filesystem/ceph-filesystem-crd.md#pools), without the filesystem name prefix | `data0` |
| `storageClass.parameters` | See [Shared Filesystem](../Storage-Configuration/Shared-Filesystem-CephFS/filesystem-storage.md) documentation or the helm values.yaml for suitable values | see values.yaml |
| `storageClass.reclaimPolicy` | The default [Reclaim Policy](https://kubernetes.io/docs/concepts/storage/storage-classes/#reclaim-policy) to apply to PVCs created with this storage class. | `Delete` |
| `storageClass.mountOptions` | Specifies the mount options for storageClass | `[]` |

### **Ceph Object Stores**

The `cephObjectStores` array in the values file will define a list of CephObjectStore as described in the table below.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `name` | The name of the CephObjectStore | `ceph-objectstore` |
| `spec` | The CephObjectStore spec, see the [CephObjectStore CRD](../CRDs/Object-Storage/ceph-object-store-crd.md) documentation. | see values.yaml |
| `storageClass.enabled` | Whether a storage class is deployed alongside the CephObjectStore | `true` |
| `storageClass.name` | The name of the storage class | `ceph-bucket` |
| `storageClass.annotations` | Additional storage class annotations | `{}` |
| `storageClass.labels` | Additional storage class labels | `{}` |
| `storageClass.parameters` | See [Object Store storage class](../Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md) documentation or the helm values.yaml for suitable values | see values.yaml |
| `storageClass.reclaimPolicy` | The default [Reclaim Policy](https://kubernetes.io/docs/concepts/storage/storage-classes/#reclaim-policy) to apply to PVCs created with this storage class. | `Delete` |
| `ingress.enabled` | Enable an ingress for the object store | `false` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.host.name` | Ingress hostname | `""` |
| `ingress.host.path` | Ingress path prefix | `/` |
| `ingress.tls` | Ingress tls | `/` |
| `ingress.ingressClassName` | Ingress tls | `""` |

### **Existing Clusters**

If you have an existing CephCluster CR that was created without the helm chart and you want the helm
chart to start managing the cluster:

1. Extract the `spec` section of your existing CephCluster CR and copy to the `cephClusterSpec`
   section in `values.yaml`.

2. Add the following annotations and label to your existing CephCluster CR:

```yaml
  annotations:
    meta.helm.sh/release-name: rook-ceph-cluster
    meta.helm.sh/release-namespace: rook-ceph
  labels:
    app.kubernetes.io/managed-by: Helm
```

1. Run the `helm install` command in the [Installing section](#release) to create the chart.

2. In the future when updates to the cluster are needed, ensure the values.yaml always
   contains the desired CephCluster spec.

### **Development Build**

To deploy from a local build from your development environment there are two steps:

1. [Deploy the operator chart](operator-chart.md#development-build), in particular to get the CRDs.
2. Deploy the cluster chart:

```console
cd deploy/charts/rook-ceph-cluster
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster -f values.yaml .
```

## Uninstalling the Chart

To see the currently installed Rook chart:

```console
helm ls --namespace rook-ceph
```

To uninstall/delete the `rook-ceph-cluster` chart:

```console
helm delete --namespace rook-ceph rook-ceph-cluster
```

The command removes all the Kubernetes components associated with the chart and deletes the release. Removing the cluster
chart does not remove the Rook operator. In addition, all data on hosts in the Rook data directory
(`/var/lib/rook` by default) and on OSD raw devices is kept. To reuse disks, you will have to wipe them before recreating the cluster.

See the [teardown documentation](../Storage-Configuration/ceph-teardown.md) for more information.
