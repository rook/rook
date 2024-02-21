# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.14

The following high level features are targeted for Rook v1.14 (April 2024). For more detailed project tracking see the [v1.14 board](https://github.com/rook/rook/projects/31).

* Support for Ceph Squid (v19)
* Allow setting the application name on a CephBlockPool [#13744](https://github.com/rook/rook/pull/13744)
* Pool sharing for multiple object stores [#11411](https://github.com/rook/rook/issues/11411)
* DNS subdomain style access to RGW buckets [#4780](https://github.com/rook/rook/issues/4780)
* Replace a single OSD when a metadataDevice is configured with multiple OSDs [#13240](https://github.com/rook/rook/issues/13240)
* Create a default service account for all Ceph daemons [#13362](https://github.com/rook/rook/pull/13362)
* Enable the rook orchestrator mgr module by default for improved dashboard integration [#13760](https://github.com/rook/rook/issues/13760)
* Option to run all components on the host network [#13571](https://github.com/rook/rook/issues/13571)
* Multus-enabled clusters to begin "holder" pod deprecation [#13055](https://github.com/rook/rook/issues/13055)
* Separate CSI image repository and tag for all images in the helm chart [#13585](https://github.com/rook/rook/issues/13585)
* Ceph-CSI [v3.11](https://github.com/ceph/ceph-csi/issues?q=is%3Aopen+is%3Aissue+milestone%3Arelease-v3.11.0)
* Add build support for Go 1.22 [#13738](https://github.com/rook/rook/pull/13738)

## Kubectl Plugin

Features are planned in the 1.14 time frame for the [Kubectl Plugin](https://github.com/rook/kubectl-rook-ceph).
* Collect details to help troubleshoot the csi driver [#69](https://github.com/rook/kubectl-rook-ceph/issues/69)
* Command to flatten an RBD image [#222](https://github.com/rook/kubectl-rook-ceph/issues/222)
