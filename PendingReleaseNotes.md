# Major Themes

v1.5...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph

- Ceph mons require an odd number for a healthy quorum. An even number of mons is now disallowed.
- Update deprecated CRD apiextensions.k8s.io/v1beta1 to v1 ([#6424](https://github.com/rook/rook/pull/6424))
- preservePoolsOnDelete is deprecated for CephFilesystem and is no longer allowed because of data-loss concerns. preserveFilesystemOnDelete acts as a replacement and preserves the filesystem when the CephFilesystem CRD is deleted (which implicitly includes all associated pools). See [#6495](https://github.com/rook/rook/pull/6495) for more details.
- The discovery daemon is disabled by default since the discovery is not necessary in most clusters. Enable the discovery daemon if
  devices are being added to nodes and you want to automatically configure OSDs on new devices without restarting the operator.
- The CRDs have been separated from common.yaml into crds.yaml to give more flexibility for upgrades.

## Features

### Core

* Discover Agent: Custom labels can be added to the DaemonSet Pods.

### Ceph

<<<<<<< HEAD
* Stretch clusters for mons and OSDs to work reliably across two datacenters (Experimental mode)
* Ceph Block Pool: add mirroring with snapshot scheduling support
* Ceph Block Pool: add `replicasPerFailureDomain` to set the number of replica in a failure domain ([#5591](https://github.com/rook/rook/issues/5591))
* Ceph Cluster: export the storage capacity of the ceph cluster ([#6475](https://github.com/rook/rook/pull/6475))
* Ceph CSI: DaemonSet and Deployment Pods can have custom labels added to them
* Ceph Cluster: add encryption support with Key Management Service
* The helm chart is updated to v3.4
  * A `crds.enabled` setting allows the CRDs to be managed separately from the helm chart
=======
* Ceph Pacific support
* Multiple Ceph Filesystems (with Pacific only)
* CephClient CRD has been converted to use the controller-runtime library
* Extending the support of vault KMS configuration for Ceph RGW
* Enable disruption budgets (PDBs) by default for Mon, RGW, MDS, and OSD daemons
* Add CephFilesystemMirror CRD to deploy cephfs-mirror daemon
* Ceph OSD: as of Nautilus 14.2.14 and Octopus 15.2.9 if the OSD scenario is simple (one OSD per disk) we won't use LVM to prepare the disk anymore
* Disable CSI GRPC metrics by default
>>>>>>> 0a81ce225... ceph: disable CSI GRPC metrics by default
