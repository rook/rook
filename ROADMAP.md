# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.18

The following high level features are targeted for Rook v1.18 (August 2025). For more detailed project tracking see the [v1.18 board](https://github.com/orgs/rook/projects/8).

* Rotate cephx keyrings automatically [#15904](https://github.com/rook/rook/issues/15904)
* Allow Rook operator to run in multiple namespaces for improved multi-cluster reconcile [#15014](https://github.com/rook/rook/issues/15014)
* Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
* CSI Driver
  * Enable the stable Ceph-CSI operator by default, after it is declared stable [#14766](https://github.com/rook/rook/issues/15271)
  * Integrate Ceph-CSI [v3.15](https://github.com/ceph/ceph-csi/milestones)

## Kubectl Plugin

Features are planned for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph), though without a committed timeline.
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
