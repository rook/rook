# Major Themes

v1.5...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph

- Ceph mons require an odd number for a healthy quorum. An even number of mons is now disallowed.

## Features

### Ceph

* Ceph Block Pool: add mirroring support
* Ceph Block Pool: add `replicasPerFailureDomain` to set the number of replica in a failure domain ([#5591](https://github.com/rook/rook/issues/5591))
* Ceph Cluster: export the storage capacity of the ceph cluster ([#6475](https://github.com/rook/rook/pull/6475))
