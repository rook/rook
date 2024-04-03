# v1.14 Pending Release Notes

## Breaking Changes

- The minimum supported version of Kubernetes is v1.25.
  Upgrade to Kubernetes v1.25 or higher before upgrading Rook.
- The Rook operator config `CSI_ENABLE_READ_AFFINITY` was removed. v1.13 clusters that have modified
  this value to be `"true"` must set the option as desired in each CephCluster as documented
  [here](https://rook.github.io/docs/rook/v1.14/CRDs/Cluster/ceph-cluster-crd/#csi-driver-options)
  before upgrading to v1.14.
- Rook is beginning the process of deprecating CSI network "holder" pods.
  If there are pods named `csi-*plugin-holder-*` in the Rook operator namespace, see the
  [detailed documentation](./Documentation/CRDs/Cluster/network-providers.md#holder-pod-deprecation)
  to disable them. This is optional for v1.14, but will be required in a future release.

## Features

- Kubernetes versions **v1.25** through **v1.29** are supported.
- Ceph daemon pods using the `default` service account now use a new `rook-ceph-default` service account.
- Allow setting the Ceph `application` on a pool
- Create object stores with shared metadata and data pools. Isolation between object stores is enabled via RADOS namespaces.
- The feature support for VolumeSnapshotGroup has been added to the RBD and CephFS CSI driver.
- Support for virtual style hosting for s3 buckets in the CephObjectStore.
- Add option to specify prefix for the OBC provisioner.
- Support Azure Key Vault for storing OSD encryption keys.
- Separate image repository and tag values in the helm chart for the CSI images.
