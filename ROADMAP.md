# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that many do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 0.9

* Update project governance policies [#1445](https://github.com/rook/rook/issues/1445)
* Add Core Infrastructure Initiative (CII) Best Practices [#1440](https://github.com/rook/rook/issues/1440)
* Integrate a more robust controller framework (e.g., CoreOS Operator SDK or Kubebuilder) [#1981](https://github.com/rook/rook/issues/1981)
* Build and integration testing improvements
  * Increase PR quality gates (e.g., [vendoring verification](https://github.com/rook/rook/issues/1822), [license scanning](https://github.com/rook/rook/issues/1993), etc.)
  * Update promotion and release channels to align with storage provider specific statuses [#1885](https://github.com/rook/rook/issues/1885)
  * Refactor test framework and helpers to support multiple storage providers [#1788](https://github.com/rook/rook/issues/1788)
  * Isolate and parallelize storage provider testing [#1218](https://github.com/rook/rook/issues/1218)
  * Longhaul testing pipeline [#1847](https://github.com/rook/rook/issues/1847)
* Custom resource validation, progress, status [#1539](https://github.com/rook/rook/issues/1539)
* Design for Volume Snapshotting and policies (consider aligning with SIG-storage) [#1552](https://github.com/rook/rook/issues/1552)
* Support for dynamic provisioning of new storage types
  * Dynamic bucket provisioning [#1705](https://github.com/rook/rook/issues/1705)
  * Dynamic database provisioning [1704](https://github.com/rook/rook/issues/1704)
* New storage providers
  * NFS operator and CRDs (backed by arbitrary PVs) [#1551](https://github.com/rook/rook/issues/1551)
  * Cassandra design [#1910](https://github.com/rook/rook/issues/1910)
* CockroachDB
  * Secure deployment using certificates [#1809](https://github.com/rook/rook/issues/1809)
  * Helm chart deployment [#1810](https://github.com/rook/rook/issues/1810)
  * Run on arbitrary PVs [#919](https://github.com/rook/rook/issues/919)
* Minio
  * Helm chart deployment [#1814](https://github.com/rook/rook/issues/1814)
  * Run on arbitrary PVs [#919](https://github.com/rook/rook/issues/919)
* Ceph
  * Update to Mimic [#1536](https://github.com/rook/rook/issues/1536)
  * Automated upgrade support (initial) [#997](https://github.com/rook/rook/issues/997)
  * Manage an existing Ceph cluster (basic) [#1868](https://github.com/rook/rook/issues/1868)
  * OSDs
    * Run on arbitrary PVs (local storage) as an alternative to host path [#796](https://github.com/rook/rook/issues/796) [#919](https://github.com/rook/rook/issues/919)
    * Minimize or eliminate running with privileged containers (e.g., reduced access to /dev) [#1944](https://github.com/rook/rook/issues/1944)
    * Disk management (adding, removing, and replacing disks) [#1435](https://github.com/rook/rook/issues/1435)
    * ceph-volume is used for provisioning [#1342](https://github.com/rook/rook/issues/1342)
  * Mgr and plugins
    * Placement group balancer support (enable the mgr module)
  * File
    * NFS Ganesha CRD [#1799](https://github.com/rook/rook/issues/1799)
    * Dynamic Volume Provisioning for CephFS [#1125](https://github.com/rook/rook/issues/1125)
  * Object
    * Multi-site configuration [#1584](https://github.com/rook/rook/issues/1584)
    * CRD for object store users [#1583](https://github.com/rook/rook/issues/1583)

## Rook 1.0

* Declare Ceph CRDs to be stable v1
* Durability of state (local storage support, config can be regenerated) [#1011](https://github.com/rook/rook/issues/1011) [#592](https://github.com/rook/rook/issues/592)
* Integration testing improvements
  * Incorporate new environments [#1315](https://github.com/rook/rook/issues/1315)
  * Incorporate more architectures [#1901](https://github.com/rook/rook/issues/1901)
* New storage providers
  * Cassandra cluster creation and horizontal scaling [#1910](https://github.com/rook/rook/issues/1910)
  * Nexenta [#1532](https://github.com/rook/rook/issues/1532)
* Ceph
  * CSI plug-in (rook-ceph-agent uses ceph-fuse, nbd-rbd / tcmu runner) [#1385](https://github.com/rook/rook/issues/1385)
  * Improved data placement and pool configuration (CRUSH maps) [#560](https://github.com/rook/rook/issues/560)
