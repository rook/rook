# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.9

The following high level features are targeted for Rook v1.9 (early April 2022). For more detailed project tracking see the [v1.9 board](https://github.com/rook/rook/projects/24).

* Support for Ceph Quincy
* Support for on-wire encryption and compression [#9054](https://github.com/rook/rook/issues/9054)
* More complete set of commands for the [Rook Krew plugin](https://github.com/rook/kubectl-rook-ceph)

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Add alpha support for COSI (Container object storage interface) with K8s 1.24 [#7843](https://github.com/rook/rook/issues/7843)
* iSCSI gateway deployment [#4334](https://github.com/rook/rook/issues/4334)
* Enable the admission controller by default [#6242](https://github.com/rook/rook/issues/6242)
* OSD encryption key rotation [#7925](https://github.com/rook/rook/issues/7925)
* Simplify metadata backup and disaster recovery
* Strengthen approach for OSDs on PVCs for a more seamless K8s management of underlying storage
* CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
  * Support for Windows nodes
