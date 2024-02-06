# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.13

The following high level features are targeted for Rook v1.13 (November 2023). For more detailed project tracking see the [v1.13 board](https://github.com/rook/rook/projects/30).

* OSD encryption on partitions [#10984](https://github.com/rook/rook/issues/10984)
* Object Store
  * Pool sharing for clusters where many object stores are required [#11411](https://github.com/rook/rook/issues/11411)
* CephFS
  * Automatic subvolume group pinning [#12607](https://github.com/rook/rook/issues/12607)
* Ceph-CSI [v3.10](https://github.com/ceph/ceph-csi/issues?q=is%3Aopen+is%3Aissue+milestone%3Arelease-v3.10.0)

## Kubectl Plugin

Features are planned in the 1.13 time frame for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph).
  * Recover the CephCluster CR after accidental deletion [#68](https://github.com/rook/kubectl-rook-ceph/issues/68)
  * Force cleanup the cluster if graceful uninstall is not desired [#131](https://github.com/rook/kubectl-rook-ceph/issues/131)
  * Provide a restricted set of commands based on a build flag [#174](https://github.com/rook/kubectl-rook-ceph/issues/174)
  * Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
