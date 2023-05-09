# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.12

The following high level features are targeted for Rook v1.12 (July 2023). For more detailed project tracking see the [v1.12 board](https://github.com/rook/rook/projects/29).

* Automate node fencing for application failover in some scenarios [#1507](https://github.com/rook/rook/issues/1507)
* OSD encryption on partitions [#10984](https://github.com/rook/rook/issues/10984)
* Support IPv6 for external clusters [#11602](https://github.com/rook/rook/issues/11602)
* Object Store
  * Add alpha support for COSI (Container object storage interface) with K8s 1.25 [#7843](https://github.com/rook/rook/issues/7843)
  * Support RGW LDAP integration [#4315](https://github.com/rook/rook/issues/4315)
  * Service account authentication with the Vault agent [#9872](https://github.com/rook/rook/pull/9872)
  * Pool sharing for clusters where many object stores are required [#11411](https://github.com/rook/rook/issues/11411)
* Ceph-CSI v3.9

## Krew Plugin

Features planned in the 1.12 time frame for the [Krew Plugin](https://github.com/rook/kubectl-rook-ceph).

  * Release a rewrite of the plugin in golang to lay the groundwork for future features [#76](https://github.com/rook/kubectl-rook-ceph/issues/76)
  * Easy command for deploying a test cluster [#95](https://github.com/rook/kubectl-rook-ceph/issues/95)

Features planned that are not yet committed in the timeline of a release.

  * Enable restoring a cluster from the OSDs after all mons are lost [#7049](https://github.com/rook/rook/issues/7049)
  * Recover the CephCluster CR after accidental deletion [#68](https://github.com/rook/kubectl-rook-ceph/issues/68)
  * Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)

## Beyond 1.12

These feature improvements to Rook are planned, though not yet committed to a release.

* CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
  * Support for Windows nodes
