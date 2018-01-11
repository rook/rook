# Major Themes

## Action Required

## Notable Features
- Monitoring is now done through the Ceph MGR service for Ceph storage.

## Breaking Changes

### Cluster CRD
- Removed the `versionTag` property. The container version to launch in all pods will be the same as the version of the operator container.
- Added the `cluster.rook.io` finalizer. When a cluster is deleted, the operator will cleanup resources and remove the finalizer, which then allows K8s to delete the CRD.

### Operator
- Removed the `ROOK_REPO_PREFIX` env var. All containers will be launched with the same image as the operator

## Known Issues

## Deprecations
- Monitoring through rook-api is deprecated. The Ceph MGR service named `rook-ceph-mgr` port `9283` path `/` should be used instead.
