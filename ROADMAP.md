# Roadmap

This document defines a high level roadmap for Rook development and upcoming releases.
The features and themes included in each milestone are optimistic in the sense that some do not have clear owners yet.
Community and contributor involvement is vital for successfully implementing all desired items for each release.
We hope that the items listed below will inspire further engagement from the community to keep Rook progressing and shipping exciting and valuable features.

Any dates listed below and the specific issues that will ship in a given milestone are subject to change but should give a general idea of what we are planning.
See the [Github project boards](https://github.com/rook/rook/projects) for the most up-to-date issues and their status.

## Rook Ceph 1.8

The following high level features are targeted for Rook v1.8 (December 2021). For more detailed project tracking see the [v1.8 board](https://github.com/rook/rook/projects/23).

* Configure bucket notifications with a CRD ([design doc](https://github.com/rook/rook/blob/master/design/ceph/object/ceph-bucket-notification-crd.md))
* Disaster Recovery (DR): CSI solution for application failover in the event of cluster failure
* OSD encryption key rotation [#7925](https://github.com/rook/rook/issues/7925)
* Kubernetes authentication for cluster-wide encryption with Vault KMS
* Provide a conversion tool for FlexVolumes to CSI and remove the FlexVolume driver support [#4043](https://github.com/rook/rook/issues/4043)
* Remove support for Nautilus, focusing on support for Octopus and Pacific [#7908](https://github.com/rook/rook/issues/7908)

## Themes

The general areas for improvements include the following, though may not be committed to a release.

* Add alpha support for COSI (Container object storage interface) with K8s 1.22 [#7843](https://github.com/rook/rook/issues/7843)
* iSCSI gateway deployment [#4334](https://github.com/rook/rook/issues/4334)
* Enable the admission controller by default [#6242](https://github.com/rook/rook/issues/6242)
* Dashboard-driven configuration after minimal CR install
* Simplify metadata backup and disaster recovery
* CSI Driver improvements tracked in the [CSI repo](https://github.com/ceph/ceph-csi)
  * Support for Windows nodes
