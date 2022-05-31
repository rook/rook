# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [GitHub project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.10

The following high level features are targeted for Rook v1.10 (early August 2022). For more detailed project tracking see the [v1.10 board](https://github.com/rook/rook/projects/26).

* Remove support for Ceph Octopus (support remains for Pacific and Quincy) [#10338](https://github.com/rook/rook/issues/10338)
* Add command to the [krew plugin](https://github.com/rook/kubectl-rook-ceph) to analyze cluster health and advise on resolving common health issues [Krew #32](https://github.com/rook/kubectl-rook-ceph/issues/32)
* Check for existing subvolumes before allowing a filesystem to be uninstalled [#9915](https://github.com/rook/rook/pull/9915)
* Use more specific cephx accounts to better differentiate the source of Ceph configuration changes [#10169](https://github.com/rook/rook/issues/10169)
* Automate node fencing for application failover in some scenarios [#1507](https://github.com/rook/rook/issues/1507)
* Object Store
  * Service account authentication with the Vault agent [#9872](https://github.com/rook/rook/pull/9872)
  * Support for AWS Server Side Encryption (SSE) [#10318](https://github.com/rook/rook/pull/10318)
  * Improvements to RGW Multisite configuration [#10323](https://github.com/rook/rook/pull/10323)


## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Add alpha support for COSI (Container object storage interface) with K8s 1.24 [#7843](https://github.com/rook/rook/issues/7843)
* OSD encryption key rotation [#7925](https://github.com/rook/rook/issues/7925)
* Simplify metadata backup and disaster recovery [#3985](https://github.com/rook/rook/issues/3985)
* Strengthen approach for OSDs on PVCs for a more seamless K8s management of underlying storage
* CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
  * Support for Windows nodes
