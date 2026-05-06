# v1.20 Pending Release Notes

## Breaking Changes

- The minimum supported Kubernetes version is v1.31.
- The CSI operator is required for managing CSI driver settings.
    - The CSI settings are removed from the Rook operator configmap and helm chart
    - New installs must configure the CSI settings with the CSI CRs instead of Rook operator settings.
    - Upgrades will continue working with the existing settings that had been applied by Rook previously. Further updates to CSI settings will need to be updated by the Rook admin.

## Features

- RGW Account support: Added `CephObjectStoreAccount` CRD for managing RGW accounts, and `accountRef` field in `CephObjectStoreUser` to associate users with accounts. This feature is experimental and currently only supported with the Ceph main branch image (`quay.ceph.io/ceph-ci/ceph:main`). See the [Object Store Accounts](Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-accounts.md) documentation for more details.
- SSE-S3 with Vault Agent: Added support for server-side encryption with SSE-S3 using HashiCorp Vault Agent authentication. See the [CephObjectStore Security Settings](Documentation/CRDs/Object-Storage/ceph-object-store-crd.md#sse-s3-with-vault-agent) for more details.
- Unused CRUSH rule cleanup: Rook now deletes unused CRUSH rules by default after the Ceph mgr starts. Set `ROOK_DELETE_UNUSED_CRUSH_RULES` to `false` in the operator config to disable this cleanup.
- Declare stable the feature to concurrently reconcile multiple Ceph Clusters with the setting `ROOK_RECONCILE_CONCURRENT_CLUSTERS`.
