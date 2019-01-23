# Major Themes

## Action Required

## Notable Features

- Different versions of Ceph can be orchestrated by Rook. Both Luminous and Mimic are now supported, with Nautilus coming soon.
  The version of Ceph is specified in the cluster CRD with the cephVersion.image property. For example, to run Mimic you could use image `ceph/ceph:v13.2.2-20181023`
  or any other image found on the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags).
- The Ceph CRDs are now v1. The operator will automatically convert the CRDs from v1beta1 to v1.
- The `fsType` default for StorageClass examples are now using XFS to bring it in line with Ceph recommendations.
- Ceph OSDs will be automatically updated by the operator when there is a change to the operator version or when the OSD configuration changes. See the [OSD upgrade notes](Documentation/upgrade-patch.md#object-storage-daemons-osds).
- Rook Ceph block storage provisioner can now correctly create erasure coded block images. See [Advanced Example: Erasure Coded Block Storage](Documentation/ceph-block.md#advanced-example-erasure-coded-block-storage) for an example usage.
- [Network File System (NFS)](https://github.com/nfs-ganesha/nfs-ganesha/wiki) is now supported by Rook with a new operator to deploy and manage this widely used server. NFS servers can be automatically deployed by creating an instance of the new `nfsservers.nfs.rook.io` custom resource. See the [NFS server user guide](Documentation/nfs.md) to get started with NFS.
- [Cassandra](http://cassandra.apache.org/) and [Scylla](https://www.scylladb.com/) are now supported by Rook with the rook-cassandra operator. Users can now deploy, configure and manage Cassandra or Scylla clusters, by creating an instance of the `clusters.cassandra.rook.io` custom resource. See the [user guide](Documentation/cassandra.md) to get started.
- Service account (`rook-ceph-mgr`) added for the mgr daemon to grant the mgr orchestrator modules access to the K8s APIs.
- The minimum version of Kubernetes supported by Rook changed from `1.7` to `1.8`.
- `reclaimPolicy` parameter of `StorageClass` definition is now supported.
- K8s client-go updated from version 1.8.2 to 1.11.3
- The toolbox manifest now creates a deployment based on the `rook/ceph` image instead of creating a pod on a specialized `rook/ceph-toolbox` image.
- The frequency of discovering devices on a node is reduced to 60 minutes by default, and is configurable with the setting `ROOK_DISCOVER_DEVICES_INTERVAL` in operator.yaml.
- The number of mons can be changed by updating the `mon.count` in the cluster CRD.
- RBD Mirroring is enabled by Rook. By setting the number of [rbd mirroring workers](Documentation/ceph-cluster-crd.md#cluster-settings), the daemon(s) will be started by rook. To configure the pools or images to be mirrored, use the Rook toolbox to run the [rbd mirror](http://docs.ceph.com/docs/mimic/rbd/rbd-mirroring/) configuration tool.
- Object Store User creation via CRD for Ceph clusters.
- Ceph OSD, MGR, MDS, and RGW deployments (or DaemonSets) will be updated/upgraded automatically with updates to the Rook operator.
- Ceph OSDs are created with the `ceph-volume` tool when configuring devices, adding support for multiple OSDs per device. See the [OSD configuration settings](Documentation/ceph-cluster-crd.md#osd-configuration-settings)
- Selinux labeling for mounts can now be toggled with the [ROOK_ENABLE_SELINUX_RELABELING](https://github.com/rook/rook/issues/2417) environment variable.
- Recursive chown for mounts can now be toggled with the [ROOK_ENABLE_FSGROUP](https://github.com/rook/rook/issues/2254) environment variable.
- Added the dashboard `port` configuration setting.
- Added the dashboard `ssl` configuration setting.

## Breaking Changes

- The Ceph CRDs are now v1. With the version change, the `kind` has been renamed for the following Ceph CRDs:
  - `Cluster` --> `CephCluster`
  - `Pool` --> `CephBlockPool`
  - `Filesystem` --> `CephFilesystem`
  - `ObjectStore` --> `CephObjectStore`
  - `ObjectStoreUser` --> `CephObjectStoreUser`
- The `rook-ceph-cluster` service account was renamed to `rook-ceph-osd` as this service account only applies to OSDs.
  - On upgrade from v0.8, the `rook-ceph-osd` service account must be created before starting the operator on v0.9.
  - The `serviceAccount` property has been removed from the cluster CRD.
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
- Upgrades are not supported to nautilus. Specifically, OSDs configured before the upgrade (without ceph-volume) will fail to start on nautilus. Nautilus is not officially supported until its release, but otherwise is expected to be working in test clusters.

## Deprecations
