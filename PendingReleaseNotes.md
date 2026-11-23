# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.

> **Do not update this file by default.** Most Rook pull requests are backported to previous
> releases, and backported changes must *not* be listed here. Only add a note when a change targets
> **this minor release only** (it is not backported) **and** is a breaking change or a notable new
> feature. Bug fixes, refactors, test changes, documentation, dependency bumps, and anything
> backported do not belong here. If you are unsure whether your change will be backported, leave this
> file unchanged — a maintainer will add a note if one is needed.

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
