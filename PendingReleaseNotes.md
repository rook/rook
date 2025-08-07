# v1.18 Pending Release Notes

## Breaking Changes

- Rook now validates node topology during CephCluster creation to prevent misconfigured CRUSH hierarchies. If child labels like `topology.rook.io/rack` are duplicated across zones, cluster creation will fail. The check applies only to new clusters without OSDs; existing clusters will log a warning and continue. To bypass, set `ROOK_SKIP_OSD_TOPOLOGY_CHECK=true` in the operator configmap. See [#16017](https://github.com/rook/rook/pull/16017) for details.

## Features

- Previously, only the latest version of helm was tested and the docs stated only version 3.x of helm as a prerequisite. Now rook supports the six most recent minor versions of helm along with their their patch updates. Explicitly, helm versions 3.13 and newer are supported.
- Add support for specifying the clusterID in the CephBlockPoolRadosNamespace and the CephFilesystemSubVolumeGroup CR.
- If a mon is being failed over, if the assigned node no longer exists, failover immediately instead of waiting for a
  20 minute timeout.
