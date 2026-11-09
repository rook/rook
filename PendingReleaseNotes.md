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
- Encrypted host-based OSDs store their keys in the KMS configured in `security.kms`, as PVC-based OSDs do.
  On clusters that already set `security.kms`, new encrypted host-based OSDs (`encryptedDevice: "true"`),
  replacements included, are created in `ceph-volume raw` mode with their keys in the KMS, so the KMS must be
  reachable for them to start and its settings must be valid. The KMS settings are now also validated for
  clusters with encrypted host-based OSDs. Existing OSDs are not changed. Encrypted partitions found with
  `useAllDevices` or listed in `devices` are now used for such OSDs. OSDs with a metadata device or
  `osdsPerDevice` greater than 1 still use `lvm` mode with keys in the monitors only.
