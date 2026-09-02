# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes

- When `network.connections.encryption.enabled` is set, Rook now also sets the mon-specific
  `ms_mon_cluster_mode`, `ms_mon_service_mode`, and `ms_mon_client_mode` settings to `secure`.
  Previously only the general `ms_*_mode` settings were set, which left mon sessions able to
  fall back to an unencrypted `crc` connection mode.

## Features
