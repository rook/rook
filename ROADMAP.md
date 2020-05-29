# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that many do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.


## Rook 1.4

The following high level features are targeted for Rook v1.4 (July 2020). For more detailed project tracking see the [v1.4 board](https://github.com/rook/rook/projects/18).

* Ceph
  * Admission controller [#4819](https://github.com/rook/rook/issues/4819)
  * RGW Multi-site replication (experimental) [#1584](https://github.com/rook/rook/issues/1584)
  * Handle IPv4/IPv6 dual stack configurations [#3850](https://github.com/rook/rook/issues/3850)
  * Support for provisioning OSDs with drive groups [#4916](https://github.com/rook/rook/pull/4916)
  * Multus networking configuration declared stable
  * RBD Mirroring configured with a CRD
  * All Ceph controllers updated to the controller runtime
  * Uninstall options for sanitizing OSD devices
  * Enhancements to external cluster configuration

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Admission Controllers
  * Improve custom resource validation for each storage provider
* Build hygiene
  * Enable security analysis tools [#4578](https://github.com/rook/rook/issues/4578)
  * Run more comprehensive tests with a daily test run [#2828](https://github.com/rook/rook/issues/2828)
  * Include more environments in the test pipeline [#1841](https://github.com/rook/rook/issues/1841)
* Controller Runtime
  * Update [remaining Rook controllers](https://github.com/rook/rook/issues?q=is%3Aissue+is%3Aopen+%22controller+runtime%22+label%3Areliability+) to build on the controller runtime
* Cassandra
  * Admission webhook to reduce user error [#2363](https://github.com/rook/rook/issues/2363)
  * Handle loss of persistent local data [#2533](https://github.com/rook/rook/issues/2533)
  * Continue to implement Cassandra design [#2294](https://github.com/rook/rook/issues/2294)
  * Enable automated repairs [#2531](https://github.com/rook/rook/issues/2531)
  * Minor version upgrades [#2532](https://github.com/rook/rook/issues/2532)
  * Graduate CRDs to beta
* Ceph
  * RGW Multi-site configurations
    * Declare the feature stable
    * Support additional scenarios
  * RBD Mirroring
    * Define CRD(s) to simplify the RBD mirroring configuration
  * Disaster Recovery (DR)
    * CSI solution for application failover in the event of cluster failure
  * Disaster Recovery (Rook)
    * Simplify metadata backup and disaster recovery [#592](https://github.com/rook/rook/issues/592)
  * Encryption
    * Data at rest encrypted for OSDs backed by PVCs
    * Encryption configuration per pool or per volume via the CSI driver
  * Helm chart for the cluster CR [#2109](https://github.com/rook/rook/issues/2109)
  * CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
    * Ceph-CSI 3.0 [features](https://github.com/ceph/ceph-csi/issues/865)
* CockroachDB
  * Helm chart deployment [#1810](https://github.com/rook/rook/issues/1810)
  * Secure deployment using certificates [#1809](https://github.com/rook/rook/issues/1809)
  * Graduate CRDs to beta
* EdgeFS
  * Cluster-wide SysRepCount support to enable single node deployments with SysRepCount=1 or 2
  * Cluster-wide FailureDomain support to enable single node deployments with FailureDomain="device"
* NFS
  * Client access control [#2283](https://github.com/rook/rook/issues/2283)
  * Dynamic NFS provisioning [#2062](https://github.com/rook/rook/issues/4982)
  * Graduate CRDs to beta
* YugabyteDB
  * Graduate CRDs to beta
