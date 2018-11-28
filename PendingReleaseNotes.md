# Major Themes

## Action Required

## Notable Features

- Different versions of Ceph can be orchestrated by Rook. Both Luminous and Mimic are now supported, with Nautilus coming soon.
  The version of Ceph is specified in the cluster CRD with the cephVersion.image property. For example, to run Mimic you could use image `ceph/ceph:v13.2.2-20181023`
  or any other image found on the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags).
- The `fsType` default for StorageClass examples are now using XFS to bring it in line with Ceph recommendations.
- Ceph OSDs will be automatically updated by the operator when there is a change to the operator version or when the OSD configuration changes. See the [OSD upgrade notes](Documentation/upgrade-patch.md#object-storage-daemons-osds).
- Rook Ceph block storage provisioner can now correctly create erasure coded block images. See [Advanced Example: Erasure Coded Block Storage](Documentation/block.md#advanced-example-erasure-coded-block-storage) for an example usage.
- [Network File System (NFS)](https://github.com/nfs-ganesha/nfs-ganesha/wiki) is now supported by Rook with a new operator to deploy and manage this widely used server. NFS servers can be automatically deployed by creating an instance of the new `nfsservers.nfs.rook.io` custom resource. See the [NFS server user guide](Documentation/nfs.md) to get started with NFS.
- The minimum version of Kubernetes supported by Rook changed from `1.7` to `1.8`.
- `reclaimPolicy` parameter of `StorageClass` definition is now supported.
- K8s client-go updated from version 1.8.2 to 1.11.3
- The toolbox manifest now creates a deployment based on the `rook/ceph` image instead of creating a pod on a specialized `rook/ceph-toolbox` image.
- The frequency of discovering devices on a node is reduced to 60 minutes by default, and is configurable with the setting `ROOK_DISCOVER_DEVICES_INTERVAL` in operator.yaml.
- The number of mons can be changed by updating the `mon.count` in the cluster CRD.
- RBD Mirroring is enabled by Rook. By setting the number of [rbd mirroring workers](Documentation/ceph-cluster-crd.md#cluster-settings), the daemon(s) will be started by rook. To configure the pools or images to be mirrored, use the Rook toolbox to run the [rbd mirror](http://docs.ceph.com/docs/mimic/rbd/rbd-mirroring/) configuration tool.
- Object Store User creation via CRD for Ceph clusters.

## Breaking Changes

- Ceph mons are [named consistently](https://github.com/rook/rook/issues/1751) with other daemons with the letters a, b, c, etc.
- Ceph mons are now created with Deployments instead of ReplicaSets to improve the upgrade implementation.
- Ceph mon, mgr, mds, and rgw container names in pods have changed with the refactors to initialize the
  daemon environments via pod **InitContainers** and run the Ceph daemons directly from the
  container entrypoint.
- The Rook container images are no longer published to quay.io, they are published only to Docker Hub.  All manifests have referenced Docker Hub for multiple releases now, so we do not expect any directly affected users from this change.
- Rook no longer supports kubernetes `1.7`. Users running Kubernetes `1.7` on their clusters are recommended to upgrade to Kubernetes `1.8` or higher. If you are using `kubeadm`, you can follow this [guide](https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/kubeadm-upgrade-1-8/) to from Kubernetes `1.7` to `1.8`. If you are using `kops` or `kubespray` for managing your Kubernetes cluster, just follow the respective projects' `upgrade` guide.
- Minio no longer exposes a configurable port for each distributed server instance to use.
  This was an internal only port that should not need to be configured by the user.
  All connections from users and clients are expected to come in through the [configurable Service instance](cluster/examples/kubernetes/minio/object-store.yaml#37).

## Known Issues

## Deprecations
