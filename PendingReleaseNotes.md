# v1.20 Pending Release Notes

## Breaking Changes

- The minimum supported Kubernetes version is v1.31.


## Features

- RGW Account support: Added `CephObjectStoreAccount` CRD for managing RGW accounts, and `accountRef` field in `CephObjectStoreUser` to associate users with accounts. This feature is experimental and currently only supported with the Ceph main branch image (`quay.ceph.io/ceph-ci/ceph:main`). See the [Object Store Accounts](Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-accounts.md) documentation for more details.
- SSE-S3 with Vault Agent: Added support for server-side encryption with SSE-S3 using HashiCorp Vault Agent authentication. See the [CephObjectStore Security Settings](Documentation/CRDs/Object-Storage/ceph-object-store-crd.md#sse-s3-with-vault-agent) for more details.
