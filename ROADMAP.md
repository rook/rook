# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.16

The following high level features are targeted for Rook v1.16 (December 2024). For more detailed project tracking see the [v1.16 board](https://github.com/orgs/rook/projects/6).

* Removed support for Ceph Quincy since at end of life [#14795](https://github.com/rook/rook/pull/14795)
* Enable mirroring for RADOS namespaces [#14701](https://github.com/rook/rook/pull/14701)
* Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
* Remove multus-enabled "holder" pods [#14289](https://github.com/rook/rook/issues/14289)
* OSD migration to enable encryption as a day 2 operation [#14719](https://github.com/rook/rook/pull/14719)
* Key rotation for Ceph object store users [#11563](https://github.com/rook/rook/issues/11563)
* CSI Driver
  * Continue integration of the new Ceph-CSI operator, targeted for stable in v1.17 [#14766](https://github.com/rook/rook/pull/14766)
  * Ceph-CSI [v3.13](https://github.com/ceph/ceph-csi/issues?q=is%3Aopen+is%3Aissue+milestone%3Arelease-v3.13.0)

## Kubectl Plugin

Features are planned for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph), though without a committed timeline.
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
