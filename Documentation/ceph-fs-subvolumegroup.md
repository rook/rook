---
title: SubVolume Group CRD
weight: 3610
indent: true
---

{% include_relative branch.liquid %}

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](quickstart.md)

# CephFilesystemSubVolumeGroup CRD

Rook allows creation of Ceph Filesystem [SubVolumeGroups](https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-subvolume-groups) through the custom resource definitions (CRDs).
Filesystem subvolume groups are an abstraction for a directory level higher than Filesystem subvolumes to effect policies (e.g., File layouts) across a set of subvolumes.
For more information about CephFS volume, subvolumegroup and subvolume refer to the [Ceph docs](https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes).

## Creating daemon

To get you started, here is a simple example of a CRD to create a subvolumegroup on the CephFilesystem "myfs".

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystemSubVolumeGroup
metadata:
  name: group-a
  namespace: rook-ceph # namespace:cluster
spec:
  # filesystemName is the metadata name of the CephFilesystem CR where the subvolume group will be created
  filesystemName: myfs
```

## Settings

If any setting is unspecified, a suitable default will be used automatically.

### CephFilesystemSubVolumeGroup metadata

- `name`: The name that will be used for the Ceph Filesystem subvolume group.

### CephFilesystemSubVolumeGroup spec

- `filesystemName`: The metadata name of the CephFilesystem CR where the subvolume group will be created.
