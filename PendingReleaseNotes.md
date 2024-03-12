# v1.14 Pending Release Notes

## Breaking Changes

- The removal of `CSI_ENABLE_READ_AFFINITY` option and its replacement with per-cluster
read affinity setting in cephCluster CR (CSIDriverOptions section) in [PR](https://github.com/rook/rook/pull/13665)
- updating `netNamespaceFilePath` for all clusterIDs in rook-ceph-csi-config configMap in [PR](https://github.com/rook/rook/pull/13613)
  - Issue: The netNamespaceFilePath isn't updated in the CSI config map for all the clusterIDs when `CSI_ENABLE_HOST_NETWORK` is set to false in `operator.yaml`
  - Impact: This results in the unintended network configurations, with pods using the host networking instead of pod networking.

## Features

- Kubernetes versions **v1.25** through **v1.29** are supported.
- Ceph daemon pods using the `default` service account now use a new `rook-ceph-default` service account.
- Allow setting the Ceph `application` on a pool
- Create object stores with shared metadata and data pools. Isolation between object stores is enabled via RADOS namespaces.
- The feature support for VolumeSnapshotGroup has been added to the RBD and CephFS CSI driver.
- Support for virtual style hosting for s3 buckets in the CephObjectStore.
- Add option to specify prefix for the OBC provisioner.
