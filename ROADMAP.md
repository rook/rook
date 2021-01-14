# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [Github project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.


## Rook 1.6

The following high level features are targeted for Rook v1.6 (March 2021). For more detailed project tracking see the [v1.6 board](https://github.com/rook/rook/projects/20).

* Ceph
  * Support the Ceph Pacific release
  * CephFS Mirroring support for data replication across clusters
  * RBD mirroring configuration between clusters to support application DR (disaster recovery)
  * RGW Multi-site replication improvements towards declaring the feature stable
  * Upgrade OSDs for large clusters in parallel within failure domains
  * Configure bucket notifications with a CRD [#6425](https://github.com/rook/rook/issues/6425)
* Storage Providers
  * Clearly define maintainer expectations for storage providers
  * Remove unmaintained storage providers: EdgeFS, CockroachDB, and YugabyteDB

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Admission Controllers
  * Improve custom resource validation for each storage provider
* Build hygiene
  * Complete conversion from Jenkins pipeline to GitHub actions
  * Run more comprehensive tests with a daily test run [#2828](https://github.com/rook/rook/issues/2828)
  * Include more environments in the test pipeline [#1841](https://github.com/rook/rook/issues/1841)
* Controller Runtime
  * Update [remaining Rook controllers](https://github.com/rook/rook/issues?q=is%3Aissue+is%3Aopen+%22controller+runtime%22+label%3Areliability+) to build on the controller runtime
* Ceph
  * Add alpha support for COSI (Container object storage interface) with K8s 1.21
  * Enable the admission controller by default [#6242](https://github.com/rook/rook/issues/6242)
  * Dashboard-driven configuration after minimal CR install
  * RGW Multi-site configurations
    * Declare the feature stable
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
