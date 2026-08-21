# v1.19 Pending Release Notes

## Breaking Changes

- The minimum supported Kubernetes version is v1.30.

- The minimum supported Ceph version is v19.2.0. Rook v1.18 clusters running Ceph v18 must upgrade
  to Ceph v19.2.0 or higher before upgrading Rook.

- The behavior of the `activeStandby` property in the `CephFilesystem` CRD has changed.
  When set to `false`, the standby MDS daemon deployment will be scaled down and removed,
  rather than only disabling the standby cache while the daemon remains running.

- Helm: The rook-ceph-cluster chart has changed where the Ceph image is defined, to allow separate
  settings for the repository and tag. For more info, see the
  [Rook upgrade guide](https://rook.io/docs/rook/v1.19/Upgrade/rook-upgrade/).

- In external mode CephClusters when users give a Ceph admin keyring to Rook, Rook will no longer
    create CSI Ceph clients itself. This makes it so that all external mode clusters are
    managed via the same mechanism (external Python script) and avoids duplicate logic issues.

## Features

<<<<<<< HEAD
- Experimental: Allow concurrent reconciles of the CephCluster CR when there multiple clusters
  being managed by the same Rook operator. Concurrency is enabled by increasing
  the operator setting `ROOK_RECONCILE_CONCURRENT_CLUSTERS` to a value greater
  than `1`.
- Improved logging with namespaced names for the controllers for more consistency in
  troubleshooting the rook operator log.
=======
- RBD QoS (Quality of Service) support via `VolumeAttributesClass` using the krbd mounter with cgroup v2 `io.max` enforcement. See the [RBD QoS documentation](Documentation/Storage-Configuration/Block-Storage-RBD/rbd-qos.md) for details.
- Automated OSD replacement. OSD deployment can be annotated to mark it for replacement. Rook will drain and destroy it with preserving its CRUSH position to later reuse it when new device will be available on the same node. All types of OSDs supported for host-based cluster included OSDs sharing metadata device. PVC-based OSDs are not supported. See [OSD replacement design document](./design/ceph/osd-replacement.md) for details.
- The rook-ceph-cluster Helm chart can create `CephObjectStoreUser` resources via the new `cephObjectStoreUsers` value.
- The toolbox deployments from the Helm chart and the example manifests now reload the keyring and `ceph.conf` automatically after CephX key rotation, mon failover, or a config override change.
>>>>>>> 365244049 (docs: note that all toolbox deployments reload the keyring)
