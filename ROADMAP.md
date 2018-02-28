# Roadmap

This document defines a high level roadmap for Rook development.
The dates below are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 0.8

* Multiple storage backends
  * Full design
  * Refactor code base and repositories to enable
  * Consider support for Minio, potentially early support for other backends time permitting
* Switch CRDs to API Aggregation
* Run on arbitrary PVs
* Remove Rook API and CLI
* Migrate CI and release pipelines to a solution hosted by the CNCF
* Run with Least Privileged and possibly without privileged containers
* Shutdown / restart issues
* Support Kubernetes 1.7+ only
* Ceph features and improvements
  * Disk management (1 OSD per pod, adding/removing disks)
  * Placement group balancer support (ceph-mgr module)
  * Mon reliability (restarts, failing over too fast, ip changes, etc.)
  * Mimic support

## Rook 0.9

* Automated upgrades
* Durability of state (local storage support, config can be regenerated)
* Custom resource validation, progress, status
* Design for Volume Snapshotting and policies (consider aligning with SIG-storage)
* Ceph features and improvements
  * Improved data placement and pool configuration (CRUSH maps)
  * Dynamic Volume Provisioning for CephFS
  * ceph-volume provisioning for OSDs
  * Ceph CSI plug-in that runs everywhere (rook-agent uses ceph-fuse, nbd-rbd / tcmu runner)
* Cluster and Pool types are declared Beta

## Rook 1.0

* Declare numerous types as v1