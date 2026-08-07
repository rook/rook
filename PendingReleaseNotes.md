# v1.21 Pending Release Notes

## Breaking Changes

- Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.

## Features

- RBD QoS (Quality of Service) support via `VolumeAttributesClass` using the krbd mounter with cgroup v2 `io.max` enforcement. See the [RBD QoS documentation](Documentation/Storage-Configuration/Block-Storage-RBD/rbd-qos.md) for details.
- Automated OSD replacement. OSD deployment can be annotated to mark it for replacement. Rook will drain and destroy it with preserving its CRUSH position to later reuse it when new device will be available on the same node. All types of OSDs supported for host-based cluster included OSDs sharing metadata device. PVC-based OSDs are not supported. See [OSD replacement design document](./design/ceph/osd-replacement.md) for details.
- `cleanupPolicy.strategy: FailOnError` makes the cluster cleanup job fail when disk sanitization fails, instead of always reporting success. The default, `BestEffort`, preserves the previous behavior. Note one change under the default: a failure to look up the physical volume of an LVM-backed OSD previously panicked the cleanup job, which surfaced as a `Failed` job; it is now logged and the job continues to the remaining OSDs, so that failure no longer fails the job under `BestEffort`.
