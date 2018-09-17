# Major Themes

## Action Required

## Notable Features

- The `fsType` default for StorageClass examples are now using XFS to bring it in line with Ceph recommendations.
- Ceph is updated from Luminous 12.2.5 to 12.2.7.
- Ceph OSDs will be automatically updated by the operator when there is a change to the operator version or when the OSD configuration changes. See the [OSD upgrade notes](Documentation/upgrade-patch.md#object-storage-daemons-osds).
- Rook Ceph block storage provisioner can now correctly create erasure coded block images. See [Advanced Example: Erasure Coded Block Storage](Documentation/block.md#advanced-example-erasure-coded-block-storage) for an example usage.
- [Network File System (NFS)](https://github.com/nfs-ganesha/nfs-ganesha/wiki) is now supported by Rook with a new operator to deploy and manage this widely used server. NFS servers can be automatically deployed by creating an instance of the new `nfsservers.nfs.rook.io` custom resource. See the [NFS server user guide](Documentation/nfs.md) to get started with NFS.
- The minimum version of Kubernetes supported by Rook changed from `1.7` to `1.8`.
- `reclaimPolicy` parameter of `StorageClass` definition is now supported. 

## Breaking Changes
- Ceph mons are [named consistently](https://github.com/rook/rook/issues/1751) with other daemons with the letters a, b, c, etc.
- Ceph mons are now created with Deployments instead of ReplicaSets to improve the upgrade implementation.
- Ceph mon container names in pods have changed with the
  [refactor](https://github.com/rook/rook/pull/2095) to initialize the mon daemon environment via
  pod **InitContainers** and run the `ceph-mon` daemon directly from the container entrypoint.

- The Rook container images are no longer published to quay.io, they are published only to Docker Hub.  All manifests have referenced Docker Hub for multiple releases now, so we do not expect any directly affected users from this change.
- Rook no longer supports kubernetes `1.7`. Users running Kubernetes `1.7` on their clusters are recommended to upgrade to Kubernetes `1.8` or higher. If you are using `kubeadm`, you can follow this [guide](https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/kubeadm-upgrade-1-8/) to from Kubernetes `1.7` to `1.8`. If you are using `kops` or `kubespray` for managing your Kubernetes cluster, just follow the respective projects' `upgrade` guide.

## Known Issues

## Deprecations
