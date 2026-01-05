# v1.19 Pending Release Notes

## Breaking Changes

- The behavior of the `activeStandby` property in the `CephFilesystem` CRD has changed.
    When set to `false`, the standby MDS daemon deployment will be scaled down and removed,
    rather than only disabling the standby cache while the daemon remains running.

- Now rook operator won't create the csi user and secrets for external mode when admin keyring is used.
  Will have a single source truth for creating it in the python script to create the ceph user and in
  the import script to create the csi k8s secrets. [PR](https://github.com/rook/rook/pull/16882)

## Features

- Experimental: Allow concurrent reconciles of the CephCluster CR when there multiple clusters
  being managed by the same Rook operator. Concurrency is enabled by increasing
  the operator setting `ROOK_RECONCILE_CONCURRENT_CLUSTERS` to a value greater
  than `1`.
- Improved logging with namespaced names for the controllers for more consistency in
  troubleshooting the rook operator log.
