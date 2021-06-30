# Major Themes

v1.7...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph
- Add user data protection when deleting Rook-Ceph Custom Resources
  - A CephCluster will not be deleted if there are any other Rook-Ceph Custom resources referencing
    it with the assumption that they are using the underlying Ceph cluster.
  - A CephObjectStore will not be deleted if there is a bucket present. In addition to protection
    from deletion when users have data in the store, this implicitly protects these resources from
    being deleted when there is a referencing ObjectBucketClaim present.
  - See [the design](https://github.com/rook/rook/blob/master/design/ceph/resource-dependencies.md)
    for detailed information.

## Features

### Core

### Ceph
