# v1.21 Pending Release Notes

## Breaking Changes

- Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.

## Features

- RBD QoS (Quality of Service) support via `VolumeAttributesClass` using the krbd mounter with cgroup v2 `io.max` enforcement. See the [RBD QoS documentation](Documentation/Storage-Configuration/Block-Storage-RBD/rbd-qos.md) for details.
- The `activate` init container of node-based OSDs now runs the rook binary (provided by a new
  `copy-bins` init container in the OSD pod) instead of an embedded shell script. Its
  device-rename fallback lists scan devices individually, so a single unreadable device no longer
  prevents an OSD from activating after a reboot renames kernel device names.
