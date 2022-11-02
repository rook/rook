# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.11

The following high level features are targeted for Rook v1.11 (January 2023). For more detailed project tracking see the [v1.11 board](https://github.com/rook/rook/projects/27).

* Use more specific cephx accounts to better differentiate the source of Ceph configuration changes [#10169](https://github.com/rook/rook/issues/10169)
* Enable restoring a cluster from the OSDs after all mons are lost [#7049](https://github.com/rook/rook/issues/7049)
* Automate node fencing for application failover in some scenarios [#1507](https://github.com/rook/rook/issues/1507)
* Support RBD mirroring across clusters with overlapping networks [#11070](https://github.com/rook/rook/issues/11070)
* OSD encryption on partitions [#10984](https://github.com/rook/rook/issues/10984)
* Object Store
  * Service account authentication with the Vault agent [#9872](https://github.com/rook/rook/pull/9872)
  * Add alpha support for COSI (Container object storage interface) with K8s 1.25 [#7843](https://github.com/rook/rook/issues/7843)
  * Support the immutable object cache [#11162](https://github.com/rook/rook/issues/11162)
* Ceph-CSI v3.8
  * CephFS encryption support [#3460](https://github.com/ceph/ceph-csi/pull/3460)
* Update the operator sdk for internal CSV generation [#10141](https://github.com/rook/rook/issues/10141)
* [Krew plugin](https://github.com/rook/kubectl-rook-ceph) features planned in the 1.11 time frame
  * Recover the CephCluster CR after accidental deletion [#68](https://github.com/rook/kubectl-rook-ceph/issues/68)
  * Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
  * Restore mon qourum from OSDs after all mons are lost [#7049](https://github.com/rook/rook/issues/7049)

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* OSD encryption key rotation [#7925](https://github.com/rook/rook/issues/7925)
* Strengthen approach for OSDs on PVCs for a more seamless K8s management of underlying storage
* CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
  * Support for Windows nodes
