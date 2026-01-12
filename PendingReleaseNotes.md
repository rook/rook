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

- Now rook operator won't create the csi user and secrets for external mode when admin keyring is used.
  There will be a single source of truth. The python script will be responsible for creating the ceph user
  and the import script will handle creating the kubernetes secrets for ceph user.  [PR](https://github.com/rook/rook/pull/16882)

## Features

- Experimental: Allow concurrent reconciles of the CephCluster CR when there multiple clusters
  being managed by the same Rook operator. Concurrency is enabled by increasing
  the operator setting `ROOK_RECONCILE_CONCURRENT_CLUSTERS` to a value greater
  than `1`.
- Improved logging with namespaced names for the controllers for more consistency in
  troubleshooting the rook operator log.
