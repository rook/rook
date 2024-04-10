---
title: FilesystemSubVolumeGroup CRD
---

!!! info
    This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](../../Getting-Started/quickstart.md)

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
  # The name of the subvolume group. If not set, the default is the name of the subvolumeGroup CR.
  name: csi
  # filesystemName is the metadata name of the CephFilesystem CR where the subvolume group will be created
  filesystemName: myfs
  # reference https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups
  # only one out of (export, distributed, random) can be set at a time
  # by default pinning is set with value: distributed=1
  # for disabling default values set (distributed=0)
  pinning:
    distributed: 1            # distributed=<0, 1> (disabled=0)
    # export:                 # export=<0-256> (disabled=-1)
    # random:                 # random=[0.0, 1.0](disabled=0.0)
  # Quota size of the subvolume group.
  #quota: 10G
  # data pool name for the subvolume group layout instead of the default data pool.
  #dataPoolName: myfs-replicated
```

## Settings

If any setting is unspecified, a suitable default will be used automatically.

### CephFilesystemSubVolumeGroup metadata

* `name`: The name that will be used for the Ceph Filesystem subvolume group.

### CephFilesystemSubVolumeGroup spec

* `name`: The spec name that will be used for the Ceph Filesystem subvolume group if not set metadata name will be used.

* `filesystemName`: The metadata name of the CephFilesystem CR where the subvolume group will be created.

* `quota`: Quota size of the Ceph Filesystem subvolume group.

* `dataPoolName`: The data pool name for the subvolume group layout instead of the default data pool.

* `pinning`: To distribute load across MDS ranks in predictable and stable ways. See the Ceph doc for [Pinning subvolume groups](https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups).
    * `distributed`: Range: <0, 1>, for disabling it set to 0
    * `export`: Range: <0-256>, for disabling it set to -1
    * `random`: Range: [0.0, 1.0], for disabling it set to 0.0

!!! note
    Only one out of (export, distributed, random) can be set at a time.
    By default pinning is set with value: `distributed=1`.
