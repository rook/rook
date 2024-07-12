# v1.15 Pending Release Notes

## Breaking Changes

- Updating Ceph COSI driver images, this impact existing COSI `Buckets` and `BucketAccesses`,
please update the `BucketClass` and `BucketAccessClass` for resolving refer [here](https://github.com/rook/rook/discussions/14297)
- During CephBlockPool updates, return an error if an invalid device class is specified. Pools with invalid device classes may start failing reconcile until the correct device class is specified. See #14057.

## Features
