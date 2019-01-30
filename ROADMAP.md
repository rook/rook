# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that many do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 1.0

* Custom resource validation, progress, status [#1539](https://github.com/rook/rook/issues/1539)
* Durability of state (local storage support, config can be regenerated) [#1011](https://github.com/rook/rook/issues/1011) [#592](https://github.com/rook/rook/issues/592)
* Integration testing improvements
  * Update promotion and release channels to align with storage provider specific statuses [#1885](https://github.com/rook/rook/issues/1885)
  * Refactor test framework and helpers to support multiple storage providers [#1788](https://github.com/rook/rook/issues/1788)
  * Isolate and parallelize storage provider testing [#1218](https://github.com/rook/rook/issues/1218)
  * Longhaul testing pipeline [#1847](https://github.com/rook/rook/issues/1847)
* Support for dynamic provisioning of new storage types
  * Dynamic bucket provisioning [#1705](https://github.com/rook/rook/issues/1705)
  * Dynamic database provisioning [1704](https://github.com/rook/rook/issues/1704)
* Cassandra
  * Admission webhook to reduce user error [#2363](https://github.com/rook/rook/issues/2363)
  * Integrate prometheus monitoring [#2530](https://github.com/rook/rook/issues/2530)
  * Integrate with Spotify Reaper to provide repairs [#2531](https://github.com/rook/rook/issues/2531)
  * Dealing with loss of persistence: leverage cassandra's mechanisms to detect when data has been lost and stream it from other nodes [#2533](https://github.com/rook/rook/issues/2533)
  * Minor version upgrades [#2532](https://github.com/rook/rook/issues/2532)
* Ceph
  * Ceph-CSI plug-in integration [#1385](https://github.com/rook/rook/issues/1385)
    * Resizing volumes [#1169](https://github.com/rook/rook/issues/1169)
  * Improved data placement and pool configuration (CRUSH maps) [#560](https://github.com/rook/rook/issues/560)
  * OSDs
    * Run on arbitrary PVs (local storage) as an alternative to host path [#919](https://github.com/rook/rook/issues/919)
    * Detect new nodes for OSD configuration [#2208](https://github.com/rook/rook/issues/2208)
    * Disk management (adding, removing, and replacing disks) [#1435](https://github.com/rook/rook/issues/1435)
  * File
    * NFS Ganesha CRD [#1799](https://github.com/rook/rook/issues/1799)
    * Dynamic Volume Provisioning for CephFS [#1125](https://github.com/rook/rook/issues/1125)
  * Object
    * Multi-site configuration [#1584](https://github.com/rook/rook/issues/1584)
  * Mgr and plugins
    * Placement group balancer support (enable the mgr module)
* CockroachDB
  * Secure deployment using certificates [#1809](https://github.com/rook/rook/issues/1809)
  * Helm chart deployment [#1810](https://github.com/rook/rook/issues/1810)
* EdgeFS
  * Declare EdgeFS CRDs to be Beta v1 [#2506](https://github.com/rook/rook/issues/2506)
  * Automatic host validation [#2409](https://github.com/rook/rook/issues/2409)
  * Target VDEVs
    * Support for flexible write I/O “sync” option [#2367](https://github.com/rook/rook/issues/2367)
  * Object
    * OpenStack/SWIFT CRD [#2509](https://github.com/rook/rook/issues/2509)
    * Support for S3 bucket as DNS subdomain [#2510](https://github.com/rook/rook/issues/2510)
  * Block (iSCSI)
    * Support Block CSI [#2507](https://github.com/rook/rook/issues/2507)
  * Mgr
    * Support for Prometheus Dashboard and REST APIs [#2401](https://github.com/rook/rook/issues/2401)
    * Support for Management GUI with automated CRD wizards [#2508](https://github.com/rook/rook/issues/2508)
* Minio
  * Helm chart deployment [#1814](https://github.com/rook/rook/issues/1814)
* NFS
  * Dynamic NFS provisioning [#2062](https://github.com/rook/rook/issues/2062)
  * Client access control [#2283](https://github.com/rook/rook/issues/2283)

## Beyond 1.0

* Support for multi-networking configurations to provide more secure storage configuration (Multus?)
* Support for more dynamic clusters such as GKE [#2107](https://github.com/rook/rook/pull/2107)
* Integration testing improvements
  * Incorporate new environments [#1315](https://github.com/rook/rook/issues/1315)
  * Incorporate more architectures [#1901](https://github.com/rook/rook/issues/1901)
* Cassandra
  * Graduate CRDs to beta
* Ceph
  * More complete upgrade automation
* CockroachDB
  * Graduate CRDs to beta
* EdgeFS
  * Graduate CRDs to v1
* Minio
  * Graduate CRDs to beta
* NFS
  * Graduate CRDs to beta
