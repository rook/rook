# Major Themes

v1.7...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph
- Add user data protection when deleting Rook-Ceph Custom Resources
  - A CephCluster will not be deleted if there are any other Rook-Ceph Custom resources referencing
    it with the assumption that they are using the underlying Ceph cluster.
  - See [the design](https://github.com/rook/rook/blob/master/design/ceph/resource-dependencies.md)
    for detailed information.

- OSDs
  - Only GPT partitions can be used for OSDs. Partitions without a type or with any other type will
    be ignored.

## Features

### Core

### Ceph
