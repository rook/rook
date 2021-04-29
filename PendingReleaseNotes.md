# Major Themes

v1.6...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph
* Support for adding OSDs via Drive Groups was removed. Please refer to the
  [Ceph upgrade guide](Documentation/ceph-upgrade.md#migrate-the-drive-group-spec) for migration
  instructions.
  See https://github.com/rook/rook/issues/7275 for more information.

## Features

### Core

### Ceph

* Ceph Pacific support and along with it:
  * Multiple Ceph Filesystems
  * Networking dualstack
* CephClient CRD has been converted to use the controller-runtime library
* Extending the support of vault KMS configuration for Ceph RGW
* Enable disruption budgets (PDBs) by default for Mon, RGW, MDS, and OSD daemons
* Add CephFilesystemMirror CRD to deploy cephfs-mirror daemon
* Multiple Ceph mgr daemons are supported for stretch clusters and other clusters where HA of the mgr is more critical
* OSDs:
  * as of Nautilus 14.2.14 and Octopus 15.2.9 if the OSD scenario is simple (one OSD per disk) we won't use LVM to prepare the disk anymore
  * for Pacific (16.2.x), Rook is able to update multiple OSD Deployments at the same time to speed
    up updates and upgrades for larger Ceph clusters
* Disable CSI GRPC metrics by default
* Update CephCSI to v3.3.0
* Monitors failover can be [disabled](Documentation/ceph-mon-health.md#failing-over-a-monitor), this is useful for monitor going under planned maintenance where automatic failover is not desired
* RGW: Rook has started using deployment's replicatset functionality instead of deploying multiple deployments for each rgw
* Support running Volume Replication Controller on the RBD provisioner pod. Depends on Cephcsi v3.3.0
* Stop standby mds agents before upgrading ceph filesystem
