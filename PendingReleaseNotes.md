# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- OSD: `wipeDevicesFromOtherClusters` now also wipes PVC-backed encrypted devices that were formatted with LUKS but failed before completion (missing `ceph_fsid` token), allowing them to be reprovisioned cleanly. Metadata and WAL PVCs as well as host-based volumes are preserved.
