# v1.21 Pending Release Notes

## Breaking Changes

- Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.
- The OSD prepare job now fails, and is retried by Kubernetes, when a freshly prepared device is
  missing from the `ceph-volume raw list` output, instead of silently reporting fewer OSDs than
  were prepared (which left OSDs registered in the osdmap with no OSD deployment created).

## Features

- RBD QoS (Quality of Service) support via `VolumeAttributesClass` using the krbd mounter with cgroup v2 `io.max` enforcement. See the [RBD QoS documentation](Documentation/Storage-Configuration/Block-Storage-RBD/rbd-qos.md) for details.
- Automated OSD replacement. OSD deployment can be annotated to mark it for replacement. Rook will drain and destroy it with preserving its CRUSH position to later reuse it when new device will be available on the same node. All types of OSDs supported for host-based cluster included OSDs sharing metadata device. PVC-based OSDs are not supported. See [OSD replacement design document](./design/ceph/osd-replacement.md) for details.
- The rook-ceph-cluster Helm chart can create `CephObjectStoreUser` resources via the new `cephObjectStoreUsers` value.
- The toolbox deployments from the Helm chart and the example manifests now reload the keyring and `ceph.conf` automatically after CephX key rotation, mon failover, or a config override change.
