# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that many do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 1.1

* Ceph
  * Remove support for Ceph Luminous
  * Stable release of Ceph-CSI plug-in (feature parity with FlexVolume)
  * Connect to an external Ceph cluster [#2175](https://github.com/rook/rook/issues/2175)
  * Mon placement respects failure domains [#2603](https://github.com/rook/rook/issues/2603)
  * User-modifiable configuration at runtime [#2470](https://github.com/rook/rook/pull/2470/files)
  * Document a safe shutdown procedure [#2517](https://github.com/rook/rook/issues/2517)
  * Support for dynamic provisioning of buckets [#1705](https://github.com/rook/rook/issues/1705)
  * K8s upgrade support based on dynamic PDBs and MDBs [#3577](https://github.com/rook/rook/issues/3577)
  * Upgrades will wait for healthy Ceph state before proceeding with each daemon restart [#2889](https://github.com/rook/rook/issues/2889)
  * Run on arbitrary PVs (e.g. local storage) as an alternative to host path
    * Support for more dynamic clusters such as GKE [#2107](https://github.com/rook/rook/issues/2107)
    * Mons and OSDs can be configured to consume PVs for persistent storage [#919](https://github.com/rook/rook/issues/919)
  * OSDs
    * Allow Ceph disk selection by full path [#1228](https://github.com/rook/rook/issues/1228)
    * Leverage node labels for CRUSH locations [#1366](https://github.com/rook/rook/issues/1366)
* EdgeFS
  * Graduate CRDs to stable v1 [#3702](https://github.com/rook/rook/issues/3702)
  * Added support for useHostLocalTime option to synchronize time in service pods to host [#3627](https://github.com/rook/rook/issues/3627)
  * Added support for Multi-homing networking to provide better storage backend security isolation [#3576](https://github.com/rook/rook/issues/3576)
  * Allow users to define Kubernetes users to define ServiceType and NodePort via the service CRD spec [#3516](https://github.com/rook/rook/pull/3516)
  * Added mgr pod liveness probes [#3492](https://github.com/rook/rook/issues/3492)
  * Ability to add/remove nodes via EdgeFS cluster CRD [#3462](https://github.com/rook/rook/issues/3462)
  * Support for device full name path spec i.e. /dev/disk/by-id/NAME [#3374](https://github.com/rook/rook/issues/3374)
  * Rolling Upgrade support [#2990](https://github.com/rook/rook/issues/2990)
  * Prevents multiple targets deployment on the same node  [#3181](https://github.com/rook/rook/issues/3181)
  * Enhance S3 compatibility support for S3X pods [#3169](https://github.com/rook/rook/issues/3169)
  * Add K8S_NAMESPACE env to EdgeFS containers [#3097](https://github.com/rook/rook/issues/3097)
  * Improved support for ISGW dynamicFetch configuring [#3070](https://github.com/rook/rook/issues/3070)
  * OLM integration [#3017](https://github.com/rook/rook/issues/3017)
* YugabyteDB
  * Create an operator to manage a YugabyteDB cluster. See the [design doc](https://github.com/rook/rook/blob/master/design/yugabyte/yugabytedb-rook-design.md)

## Future improvements

* Custom resource validation, progress, status [#1539](https://github.com/rook/rook/issues/1539)
* Integration testing improvements
  * Update promotion and release channels to align with storage provider specific statuses [#1885](https://github.com/rook/rook/issues/1885)
  * Speed up integration tests [#1218](https://github.com/rook/rook/issues/1218)
  * Longhaul testing pipeline [#1847](https://github.com/rook/rook/issues/1847)
  * Include more architectures [#1901](https://github.com/rook/rook/issues/1901)
    and environments [#1315](https://github.com/rook/rook/issues/1315)
                     [#1841](https://github.com/rook/rook/issues/1841) in test pipeline
* Support for dynamic provisioning of database [#1704](https://github.com/rook/rook/issues/1704) storage types
* Update Rook controllers to build on the controller runtime [#1981](https://github.com/rook/rook/issues/1981)
* Wildcard support for disk selection spec [#1744](https://github.com/rook/rook/issues/1744)
* Cassandra
  * Admission webhook to reduce user error [#2363](https://github.com/rook/rook/issues/2363)
  * Handle loss of persistent local data [#2533](https://github.com/rook/rook/issues/2533)
  * Continue to implement Cassandra design [#2294](https://github.com/rook/rook/issues/2294)
  * Integrate prometheus monitoring [#2530](https://github.com/rook/rook/issues/2530)
  * Enable automated repairs [#2531](https://github.com/rook/rook/issues/2531)
  * Minor version upgrades [#2532](https://github.com/rook/rook/issues/2532)
  * Graduate CRDs to beta
* Ceph
  * Support for multi-networking configurations to provide more secure storage configuration. [#2621](https://github.com/rook/rook/issues/2621)
  * Document metadata backup and disaster recovery [#592](https://github.com/rook/rook/issues/592)
  * Orchestrate multi-site replication [#1584](https://github.com/rook/rook/issues/1584)
* CockroachDB
  * Helm chart deployment [#1810](https://github.com/rook/rook/issues/1810)
  * Secure deployment using certificates [#1809](https://github.com/rook/rook/issues/1809)
  * Graduate CRDs to beta
* EdgeFS
* Minio
  * Helm chart deployment [#1814](https://github.com/rook/rook/issues/1814)
  * End-to-end integration tests [#1804](https://github.com/rook/rook/issues/1804)
  * Graduate CRDs to beta
* NFS
  * Client access control [#2283](https://github.com/rook/rook/issues/2283)
  * Dynamic NFS provisioning [#2062](https://github.com/rook/rook/issues/2062)
  * Graduate CRDs to beta
* YugabyteDB
