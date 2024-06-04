# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.15

The following high level features are targeted for Rook v1.15 (July 2024). For more detailed project tracking see the [v1.15 board](https://github.com/rook/rook/projects/32).

* Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
* Multus-enabled clusters will potentially remove "holder" pods [#14289](https://github.com/rook/rook/issues/14289)
* Key rotation for Ceph object store users [#11563](https://github.com/rook/rook/issues/11563)
* CSI Driver
  * Integrate the new Ceph-CSI operator [#14260](https://github.com/rook/rook/issues/14260)
  * Ceph-CSI [v3.12](https://github.com/ceph/ceph-csi/issues?q=is%3Aopen+is%3Aissue+milestone%3Arelease-v3.12.0)
  * Support log rotation for the Ceph-CSI pods [#12809](https://github.com/rook/rook/issues/12809)

## Kubectl Plugin

Features are planned for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph), though without a committed timeline.
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
* Support `radosgw-admin` commands from the plugin [#253](https://github.com/rook/kubectl-rook-ceph/issues/253)
