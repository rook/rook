# Roadmap

This document defines a high level roadmap for Rook development.
The dates below are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 0.8

* Multiple storage backends
  * Full design
  * Refactor code base and repositories to enable
  * Consider support for Minio, potentially early support for other backends time permitting
* Run on arbitrary PVs
* Remove Rook API and CLI
* Run with Least Privileged design
  * Improved support for OpenShift
* Support Kubernetes 1.7+ only
* Ceph features and improvements
  * Disk management (1 OSD per pod, adding/removing disks)
  * Placement group balancer support (ceph-mgr module)
  * Mon reliability (one mon per node, failing over too fast, etc.)

## Rook 0.9

* Durability of state (local storage support, config can be regenerated)
* Custom resource validation, progress, status
* Design for Volume Snapshotting and policies (consider aligning with SIG-storage)
* Run without privileged containers
* Ceph features and improvements
  * Mimic support
  * Improved data placement and pool configuration (CRUSH maps)
  * Dynamic Volume Provisioning for CephFS
  * ceph-volume provisioning for OSDs
  * Ceph CSI plug-in that runs everywhere (rook-ceph-agent uses ceph-fuse, nbd-rbd / tcmu runner)
* Cluster and Pool types are declared Beta

## Rook 1.0

* Declare numerous types as v1
* Automated upgrades
