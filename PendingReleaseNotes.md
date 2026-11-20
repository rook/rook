# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- `CephObjectStoreUser` gained `spec.secretTemplate`, which adds labels and annotations to the Secret that holds the
  user's credentials, for example to opt the Secret in to a tool that copies Secrets into other namespaces. Keys that
  Rook sets or acts on are reserved.
- RGW SSE-S3 accepts `VAULT_TRANSIT_BACKEND_PATH` in `security.s3.connectionDetails` for a Vault transit engine that is
  not mounted at the default `transit` path. RGW SSE-KMS with the `transit` engine accepts it in
  `security.kms.connectionDetails` too.
- When updating OSDs in parallel, Rook now restricts each batch to a single bucket of the minimum failure domain of the
  pools defined by CephBlockPools, CephFilesystems and CephObjectStores (typically `host`), which is also the failure
  domain that the OSD PDBs allow to be disrupted. Batches may be smaller than before, in particular for OSDs on portable
  PVCs, which each have their own `host` bucket. Set `storage.osdUpdateFailureDomain` to choose another failure domain,
  or to `root` to only restrict batches to a single CRUSH root, which in most clusters matches the previous behavior.
- The OSD PDBs now count pools without a `failureDomain` or with the `osd` failure domain as `host`, and take into
  account the `subFailureDomain` of replicated pools with more than one replica per failure domain. In clusters with
  such pools, for example stretch clusters, the PDBs may allow fewer OSDs to be disrupted at the same time.
