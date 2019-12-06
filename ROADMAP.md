# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that many do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 1.2

* Integration Tests
  * Speed up integration tests [#1218](https://github.com/rook/rook/issues/1218)
* Ceph
  * Handle IPv4/IPv6 dual stack configurations [#3850](https://github.com/rook/rook/issues/3850)
  * Simplify metadata backup and disaster recovery [#592](https://github.com/rook/rook/issues/592)
  * Add support for priority classes [#2787](https://github.com/rook/rook/issues/2787)
  * Allow fine-grained management of cephx users with a Client CRD [#3175](https://github.com/rook/rook/issues/3175)
  * Gather Ceph crash reports automatically, to be viewed in the Ceph dashboard [#2882](https://github.com/rook/rook/issues/2882)
  * Expose a Ceph Client CRD to create capabilities for external usage [#3175](https://github.com/rook/rook/issues/3175)
  * Update the Ceph-CSI driver to the latest version (tentatively v2.0)
* EdgeFS
  * Improvements for single node clusters
  * Add support for payloads on external S3 server [#4431](https://github.com/rook/rook/issues/4431)
  * Add support for rtkvs disk engine and Samsung KVSSD [#3997](https://github.com/rook/rook/issues/3997)
* YugabyteDB

## Future improvements

* Formalize the Rook API
* Custom resource validation, progress, status [#1539](https://github.com/rook/rook/issues/1539)
* Integration testing improvements
  * Update promotion and release channels to align with storage provider specific statuses [#1885](https://github.com/rook/rook/issues/1885)
  * Include more environments in test pipeline [#1841](https://github.com/rook/rook/issues/1841)
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
  * Support for multi-networking (Multus) to provide more secure storage configuration [#4077](https://github.com/rook/rook/pull/4077)
  * Orchestrate multi-site replication [#1584](https://github.com/rook/rook/issues/1584)
  * OSD on PVC support for different data and metadata PVCs [#3852](https://github.com/rook/rook/issues/3852)
  * Support for Ceph Octopus
* CockroachDB
  * Helm chart deployment [#1810](https://github.com/rook/rook/issues/1810)
  * Secure deployment using certificates [#1809](https://github.com/rook/rook/issues/1809)
  * Graduate CRDs to beta
* EdgeFS
  * ISGW support for multi-region configurations [#4293](https://github.com/rook/rook/issues/4293)
  * Cluster-wide SysRepCount support to enable single node deployments with SysRepCount=1 or 2
  * Cluster-wide FailureDomain support to enable single node deployments with FailureDomain="device"
  * Support for payload on external S3 server [#4431](https://github.com/rook/rook/issues/4431)
  * Support for rtkvs disk engine and Samsung KVSSD [#3997](https://github.com/rook/rook/issues/3997)
* NFS
  * Client access control [#2283](https://github.com/rook/rook/issues/2283)
  * Dynamic NFS provisioning [#2062](https://github.com/rook/rook/issues/2062)
  * Graduate CRDs to beta
* YugabyteDB
