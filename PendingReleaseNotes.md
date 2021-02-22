# Major Themes

v1.6...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph
* Support for adding OSDs via Drive Groups was removed. Please refer to the 
  [Ceph upgrade guide](Documentation/ceph-upgrade.md#migrate-the-drive-group-spec) for migration
  instructions.
  See https://github.com/rook/rook/issues/7275 for more information.

## Features

### Core

### Ceph

* CephClient CRD has been converted to use the controller-runtime library
* Extending the support of vault KMS configuration for Ceph RGW
* Enable disruption budgets (PDBs) by default for Mon, RGW, MDS, and OSD daemons
* Add CephFilesystemMirror CRD to deploy cephfs-mirror daemon
