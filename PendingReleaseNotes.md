# v1.15 Pending Release Notes

## Breaking Changes

- Updating Ceph COSI driver images, this impact existing COSI `Buckets` and `BucketAccesses`,
please update the `BucketClass` and `BucketAccessClass` for resolving refer [here](https://github.com/rook/rook/discussions/14297)
- During CephBlockPool updates, return an error if an invalid device class is specified. Pools with invalid device classes may start failing reconcile until the correct device class is specified. See #14057.
- CephObjectStore, CephObjectStoreUser, and OBC endpoint behavior has changed when CephObjectStore
  `spec.hosting` configurations are set. A new `spec.hosting.advertiseEndpoint` config was added to
  allow users to define required behavior.

## Features

- Added support for Ceph Squid (v19)
- Allow updating the device class of OSDs, if `allowDeviceClassUpdate: true` is set
- Support for keystone authentication for s3 and swift (see [#9088](https://github.com/rook/rook/issues/9088)).