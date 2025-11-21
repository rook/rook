# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.19

The following high level features are targeted for Rook v1.19 (December 2025). For more detailed project tracking see the [v1.19 board](https://github.com/orgs/rook/projects/11).

* Allow Rook to reconcile multiple ceph clusters in parallel [#15014](https://github.com/rook/rook/pull/16719)
* Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
* Support creation of Ceph OSDs with Ceph seastore when available in raw mode [#16678](https://github.com/rook/rook/issues/16678)
* Add (experimental) support for NVME-oF available with the Ceph Tentacle release [#15551](https://github.com/rook/rook/issues/15551)
* Ceph version support
    * Set Ceph Tentacle v20 as the default version [#16747](https://github.com/rook/rook/issues/16747)
    * Remove support for Ceph Reef v18 [#16753](https://github.com/rook/rook/issues/16753)
* CSI Driver
  * Integrate Ceph-CSI [v3.16](https://github.com/ceph/ceph-csi/milestones)

## Kubectl Plugin

Features are planned for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph), though without a committed timeline.
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
