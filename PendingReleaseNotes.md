# v1.21 Pending Release Notes

## Breaking Changes

- Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.

## Features

- RBD QoS (Quality of Service) support via `VolumeAttributesClass` using the krbd mounter with cgroup v2 `io.max` enforcement. See the [RBD QoS documentation](Documentation/Storage-Configuration/Block-Storage-RBD/rbd-qos.md) for details.
- CephCluster dashboard TLS certificates can now be configured from a same-namespace Kubernetes TLS Secret with `spec.dashboard.sslCertificateRef` when dashboard SSL is enabled. Rook reconciles updates to the referenced Secret and restores the default self-signed certificate when the reference is removed.
