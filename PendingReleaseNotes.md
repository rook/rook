# Major Themes

## Action Required

## Notable Features

- The `fsType` default for StorageClass examples are now using XFS to bring it in line with Ceph recommendations.
- Ceph is updated from Luminous 12.2.5 to 12.2.7.
- Ceph OSDs will be automatically updated by the operator when there is a change to the operator version or when the OSD configuration changes. See the [OSD upgrade notes](Documentation/upgrade-patch.md#object-storage-daemons-osds).
- Rook Ceph block storage provisioner can now correctly create erasure coded block images. See [Advanced Example: Erasure Coded Block Storage](Documentation/block.md#advanced-example-erasure-coded-block-storage) for an example usage.

## Breaking Changes

- The Rook container images are no longer published to quay.io, they are published only to Docker Hub.  All manifests have referenced Docker Hub for multiple releases now, so we do not expect any directly affected users from this change.

## Known Issues

## Deprecations
