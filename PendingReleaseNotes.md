# v1.15 Pending Release Notes

## Breaking Changes

- Rook has deprecated CSI network "holder" pods.
    If there are pods named `csi-*plugin-holder-*` in the Rook operator namespace, see the
    [detailed documentation](../CRDs/Cluster/network-providers.md#holder-pod-deprecation)
    to disable them. This deprecation process is required before upgrading to the future Rook v1.16.
- Ceph COSI driver images have been updated. This impacts existing COSI Buckets, BucketClaims, and
    BucketAccesses. Update existing clusters following the guide
    [here](https://github.com/rook/rook/discussions/14297).
- During CephBlockPool updates, Rook will now return an error if an invalid device class is
    specified. Pools with invalid device classes may start failing until the correct device class is
    specified. For more info, see [#14057](https://github.com/rook/rook/pull/14057).
- CephObjectStore, CephObjectStoreUser, and OBC endpoint behavior has changed when CephObjectStore
    `spec.hosting` configurations are set. Use the new `spec.hosting.advertiseEndpoint` config to
    define required behavior as
    [documented](../Storage-Configuration/Object-Storage-RGW/object-storage.md#object-store-endpoint).
- Minimum version of Kubernetes supported is increased to K8s v1.26.

## Features

- Added support for Ceph Squid (v19)
- Allow updating the device class of OSDs, if `allowDeviceClassUpdate: true` is set
- CephObjectStore support for keystone authentication for S3 and Swift
    (see [#9088](https://github.com/rook/rook/issues/9088)).
- Support K8s versions v1.26 through v1.31.
- Use fully-qualified image names (`docker.io/rook/ceph`) in operator manifests and helm charts
