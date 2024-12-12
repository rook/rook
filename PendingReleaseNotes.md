# v1.16 Pending Release Notes

## Breaking Changes

- Removed support for Ceph Quincy (v17) since it has reached end of life
- Minimum K8s version updated to v1.27

## Features

- Added supported for K8s version v1.32
- Enable mirroring for CephBlockPoolRadosNamespaces (see [#14701](https://github.com/rook/rook/pull/14701)).
- Enable periodic monitoring for CephBlockPoolRadosNamespaces mirroring (see [#14896](https://github.com/rook/rook/pull/14896)).
- Allow migration of PVC based OSDs to enable or disable encryption (see [#14776](https://github.com/rook/rook/pull/14776)).
- Support `rgw_enable_apis` option for CephObjectStore (see [#15064](https://github.com/rook/rook/pull/15064)).
- ObjectBucketClaim management of s3 bucket policy via the `bucketPolicy` field (see [#15138](https://github.com/rook/rook/pull/15138)).
