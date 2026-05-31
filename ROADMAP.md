# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.20

The following high level features are targeted for Rook v1.20 (May 2026). For more detailed project tracking see the [v1.20 board](https://github.com/orgs/rook/projects/13).

* Disable msgr1 protocol by default [#17081](https://github.com/rook/rook/issues/17081)
* Support two-node fenced clusters [#17175](https://github.com/rook/rook/pull/17175)
* Object store
  * User Account CRD for managing RGW user accounts [#16763](https://github.com/rook/rook/issues/16763)
  * Migrate to the AWS SDK v2 [#14869](https://github.com/rook/rook/issues/14869)
* CSI Driver
  * CSI operator management configured by the user instead of legacy Rook operator settings [#16561](https://github.com/rook/rook/issues/16561)
  * Integrate Ceph-CSI [v3.17](https://github.com/ceph/ceph-csi/milestones)
* OSDs
  * Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
  * Support creation of Ceph OSDs with Ceph seastore when available in raw mode [#16678](https://github.com/rook/rook/issues/16678)

## Kubectl Plugin

Features are planned for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph), though without a committed timeline.
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
