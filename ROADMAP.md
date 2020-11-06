# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
We use the [project boards](https://github.com/rook/rook/projects) in Github so look there for the most up-to-date issues and their status.


## Rook 1.5

The following high level features are targeted for Rook v1.5 (November 2020). For more detailed project tracking see the [v1.5 board](https://github.com/rook/rook/projects/19).

* Ceph
  * RBD mirroring configuration between clusters to support application DR (disaster recovery)
  * Support for an external KMS (e.g. Vault) for storing OSD encryption keys [#6105](https://github.com/rook/rook/issues/6105)
  * Support for stretched clusters with an arbiter mon [#5592](https://github.com/rook/rook/issues/5592)
  * Encryption configuration per pool or per volume via the CSI driver
  * RGW Multi-site replication improvements towards declaring the feature stable
  * Multus networking configuration declared stable
  * Support IPv6 single stack configuration [#3850](https://github.com/rook/rook/issues/3850)
  * Enabling the admission controller by default [#6242](https://github.com/rook/rook/issues/6242)
* NFS
  * A quota for disk usage can now be set and enforced for each NFS PV [#5788](https://github.com/rook/rook/issues/5788)
  * NFS PV's can be deleted and cleaned up [#3074](https://github.com/rook/rook/issues/3074)
  * Isolation is enforced between each PV provisioned for a given NFS export: [#4982](https://github.com/rook/rook/issues/4982)

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Admission Controllers
  * Improve custom resource validation for each storage provider
* Build hygiene
  * Run more comprehensive tests with a daily test run [#2828](https://github.com/rook/rook/issues/2828)
  * Include more environments in the test pipeline [#1841](https://github.com/rook/rook/issues/1841)
* Controller Runtime
  * Update [remaining Rook controllers](https://github.com/rook/rook/issues?q=is%3Aissue+is%3Aopen+%22controller+runtime%22+label%3Areliability+) to build on the controller runtime
* Ceph
  * RGW Multi-site configurations
    * Declare the feature stable
    * Support additional scenarios
  * Disaster Recovery (DR)
    * CSI solution for application failover in the event of cluster failure
  * Disaster Recovery (Rook)
    * Simplify metadata backup and disaster recovery [#592](https://github.com/rook/rook/issues/592)
  * Helm chart for the cluster CR [#2109](https://github.com/rook/rook/issues/2109)
  * CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
* Cassandra
  * Handle loss of persistent local data [#2533](https://github.com/rook/rook/issues/2533)
  * Enable automated repairs [#2531](https://github.com/rook/rook/issues/2531)
  * Graduate CRDs to beta
