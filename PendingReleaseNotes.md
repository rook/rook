# v1.20 Pending Release Notes

## Breaking Changes

- The minimum supported Kubernetes version is v1.31.
- Change the default Ceph msgr protocol to v2. Until this release, both v1 (port 6789) and v2 (port 3300)
  have been enabled on the mon daemons. The v1 port is obsolete, highly recommended to move to v2 and
  consider enabling security features such as encryption on the wire. Requires kernel 5.11.
  If needed, it is still possible to disable this setting and enable the v1 protocol.
   - Internal clusters: In the CephCluster CR: `network.connections.requireMsgr2: false`
   - External clusters: `--v2-port-enable=False`

## Features

- RGW Account support: Added `CephObjectStoreAccount` CRD for managing RGW accounts, and `accountRef` field in `CephObjectStoreUser` to associate users with accounts. This feature is experimental and currently only supported with the Ceph main branch image (`quay.ceph.io/ceph-ci/ceph:main`). See the [Object Store Accounts](Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-accounts.md) documentation for more details.
- SSE-S3 with Vault Agent: Added support for server-side encryption with SSE-S3 using HashiCorp Vault Agent authentication. See the [CephObjectStore Security Settings](Documentation/CRDs/Object-Storage/ceph-object-store-crd.md#sse-s3-with-vault-agent) for more details.
