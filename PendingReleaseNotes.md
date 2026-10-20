# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- `CephObjectStoreUser` gained `spec.secretTemplate`, which adds labels and annotations to the Secret that holds the
  user's credentials, for example to opt the Secret in to a tool that copies Secrets into other namespaces. Keys that
  Rook sets or acts on are reserved.
- RGW SSE-S3 now honors `VAULT_BACKEND_PATH` as the mount path of the Vault transit engine. Object stores that already set `VAULT_BACKEND_PATH` under `security.s3.connectionDetails` while the engine is mounted at `transit` must remove it or change it to the real mount path.
