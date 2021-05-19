# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [Github project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.


## Rook 1.7

The following high level features are targeted for Rook v1.7 (July 2021). For more detailed project tracking see the [v1.7 board](https://github.com/rook/rook/projects/21).

* Ceph
  * Helm chart for the cluster CR [#2109](https://github.com/rook/rook/issues/2109)
  * Configure bucket notifications with a CRD ([design doc](https://github.com/rook/rook/blob/master/design/ceph/object/ceph-bucket-notification-crd.md))
  * Add alpha support for COSI (Container object storage interface) with K8s 1.22 [#7843](https://github.com/rook/rook/issues/7843)
  * Disaster Recovery (DR): CSI solution for application failover in the event of cluster failure
  * Allow OSDs on PVCs to automatically grow when the cluster is nearly full [#6101](https://github.com/rook/rook/issues/6101)
  * OSD encryption key rotation [#7925](https://github.com/rook/rook/issues/7925)
  * iSCSI gateway deployment [#4334](https://github.com/rook/rook/issues/4334)
  * Use go-ceph to interact with object store instead of `radosgw-admin` [#7924](https://github.com/rook/rook/issues/7924)
  * RGW Multi-site replication improvements towards declaring the feature stable [#6401](https://github.com/rook/rook/issues/6401)
  * More complete solution for protecting against accidental cluster deletion [#7885](https://github.com/rook/rook/pull/7885)
  * Remove support for Nautilus, focusing on support for Octopus and Pacific [#7908](https://github.com/rook/rook/issues/7908)
 * Build hygiene
  * Complete conversion from Jenkins pipeline to GitHub actions

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Admission Controllers
  * Improve custom resource validation for each storage provider
* Controller Runtime
  * Update [remaining Rook controllers](https://github.com/rook/rook/issues?q=is%3Aissue+is%3Aopen+%22controller+runtime%22+label%3Areliability+) to build on the controller runtime
* Ceph
  * Enable the admission controller by default [#6242](https://github.com/rook/rook/issues/6242)
  * Dashboard-driven configuration after minimal CR install
  * Simplify metadata backup and disaster recovery
  * CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
    * Support for Windows nodes
* Cassandra
  * Handle loss of persistent local data [#2533](https://github.com/rook/rook/issues/2533)
  * Enable automated repairs [#2531](https://github.com/rook/rook/issues/2531)
  * Graduate CRDs to beta
* NFS
  * Graduate CRDs to beta
