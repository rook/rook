# Major Themes

## Action Required

## Notable Features
- The Cluster CRD can now be edited/updated to add and remove storage nodes.  Note that only adding/removing entire nodes is currently supported, but adding individual disks/directories will also be supported soon.
- Monitoring is now done through the Ceph MGR service for Ceph storage.
- The CRUSH root can be specified for pools with the `crushRoot` property, rather than always using the `default` root. Configuration of the CRUSH hierarchy is necessary with the `ceph osd crush` commands in the toolbox.

### Operator Settings
- `AGENT_TOLERATION`: Toleration can be added to the Rook agent, such as to run on the master node.
- `FLEXVOLUME_DIR_PATH`: Flex volume directory can be overridden on the Rook agent.

### Object Store
- The dns host name can be set on the object store. See the `host` property on the [object store gateway settings](https://rook.io/docs/rook/master/object-store-crd.html#gateway-settings).

## Breaking Changes
- `armhf` build of Rook have been removed. Ceph is not supported or tested on `armhf`. arm64 support continues.

### Cluster CRD
- Removed the `versionTag` property. The container version to launch in all pods will be the same as the version of the operator container.
- Added the `cluster.rook.io` finalizer. When a cluster is deleted, the operator will cleanup resources and remove the finalizer, which then allows K8s to delete the CRD.

### Operator
- Removed the `ROOK_REPO_PREFIX` env var. All containers will be launched with the same image as the operator

## Known Issues

## Deprecations
- Monitoring through rook-api is deprecated. The Ceph MGR service named `rook-ceph-mgr` port `9283` path `/` should be used instead.
