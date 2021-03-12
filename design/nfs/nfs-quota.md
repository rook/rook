# NFS Quota

## Background

Currently, when the user creates NFS PersistentVolumes from an NFS Rook share/export via PersistentVolumeClaim, the provisioner does not provide the specific capacity as requested. For example the users create NFS PersistentVolumes via PersistentVolumeClaim as following:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rook-nfs-pv-claim
spec:
  storageClassName: "rook-nfs-share"
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Mi
```

The client still can use the higher capacity than `1mi` as requested.

This proposal is to add features which the Rook NFS Provisioner will provide the specific capacity as requested from `.spec.resources.requests.storage` field in PersistentVolumeClaim.

## Implementation

The implementation will be use `Project Quota` on xfs filesystem. When the users need to use the quota feature they should use xfs filesystem with `prjquota/pquota` mount options for underlying volume. Users can specify filesystem type and mount options through StorageClass that will be used for underlying volume. For example:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard-xfs
parameters:
  fsType: xfs
mountOptions:
  - prjquota
...
```

> Note: Many distributed storage providers for Kubernetes support xfs filesystem. Typically by defining `fsType: xfs` or `fs: xfs` (depend on storage providers) in storageClass parameters. for more detail about specify filesystem type please see https://kubernetes.io/docs/concepts/storage/storage-classes/

Then the underlying PersistentVolumeClaim should be using that StorageClass

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-default-claim
spec:
  storageClassName: "standard-xfs"
  accessModes:
  - ReadWriteOnce
...
```

If the above conditions are met then the Rook NFS Provisioner will create projects and set the quota limit using [xfs_quota](https://linux.die.net/man/8/xfs_quota) before creating PersistentVolumes based on `.spec.resources.requests.storage` field in PersistentVolumeClaim. Otherwise the Rook NFS Provisioner will provision a PersistentVolumes without creating setting the quota.

To creating the project, Rook NFS Provisioner will invoke the following command

> xfs_quota -x -c project -s -p '*nfs_pv_directory* *project_id*' *projects_file*

And setting quota with the command

> xfs_quota -x -c 'limit -p bhard=*size* *project_id*' *projects_file*

which

1. *nfs_pv_directory* is sub-directory from exported directory that used for NFS PV.
1. *project_id* is unique id `uint16` 1 to 65535.
1. *size* is size of quota as requested.
1. *projects_file* is file that contains *project quota block* for persisting quota state purpose. In case the Rook NFS Provisioner pod is killed, Rook NFS Provisioner pod will restore the quota state based on *project quota block* entries in *projects_file* at startup.
1. *project quota block* is combine of *project_id*:*nfs_pv_directory*:*size*

Since Rook NFS has the ability to create more than one NFS share/export that have different underlying volume directories, the *projects_file* will be saved on each underlying volume directory. So each NFS share/export will have different *projects_file* and each *project_file* will be persisted. The *projects_file* will only be created if underlying volume directory is mounted as `xfs` with `prjquota` mount options. This mean the existence of *project_file* will indicate if quota was enabled. The hierarchy of directory will look like:

```text
/
├── underlying-volume-A (export A) (mounted as xfs with prjquota mount options)
│   ├── projects_file
│   ├── nfs-pv-a (PV-A) (which quota created for)
│   │   ├── data (from PV-A)
│   └── nfs-pv-b (PV-B) (which quota created for)
│       └── data (from PV-B)
├── underlying-volume-B (export B) (mounted as xfs with prjquota mount options)
│   ├── projects_file
│   └── nfs-pv-c (PV-C) (which quota created for)
└── underlying-volume-C (export C) (not mounted as xfs)
    └── nfs-pv-d (PV-D) (quota not created)
```

The hierarchy above is example Rook NFS has 3 nfs share/exports (A, B and C). *project_file* inside underlying-volume-A will contains *project quota block* like

```
1:/underlying-volume-A/nfs-pv-a:size
2:/underlying-volume-A/nfs-pv-b:size
```

*project_file* inside underlying-volume-B will look like

```
1:/underlying-volume-B/nfs-pv-c:size
```

underlying-volume-C not have *project_file* because it is not mounted as xfs filesystem.

### Updating container image

Since `xfs_quota` binary is not installed by default we need to update Rook NFS container image by installing `xfsprogs` package.

### Why XFS

Most of Kubernetes VolumeSource use ext4 filesystem type if `fsType` is unspecified by default. Ext4 also have project quota feature starting in [Linux kernel 4.4](https://lwn.net/Articles/671627/). But not like xfs which natively support project quota, to mount ext4 with prjquota option we need additional step such as enable the project quota through [tune2fs](https://linux.die.net/man/8/tune2fs) before it mounted and some linux distro need additional kernel module for quota management. So for now we will only support xfs filesystem when users need quota feature in Rook NFS and might we can expand to ext4 filesystem also if possible.

## References

1. https://kubernetes.io/docs/concepts/storage/volumes/
1. https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/
1. https://linux.die.net/man/8/xfs_quota
1. https://lwn.net/Articles/671627/
1. https://linux.die.net/man/8/tune2fs
1. https://www.digitalocean.com/community/tutorials/how-to-set-filesystem-quotas-on-ubuntu-18-04#step-2-%E2%80%93-installing-the-quota-kernel-module
