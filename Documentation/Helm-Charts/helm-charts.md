---
title: Helm Charts Overview
---

The following charts are available to configure Ceph storage:

1. [Rook Ceph Operator](operator-chart.md): Starts the Ceph Operator, which will watch for Ceph CRs (custom resources). Also installs the Ceph-CSI operator as a Helm dependency.
1. [Ceph-CSI drivers chart](csi-drivers-chart.md): Installs the Ceph-CSI drivers to provision and mount volumes.
1. [Rook Ceph Cluster](ceph-cluster-chart.md): Creates Ceph CRs that the operator will use to configure the cluster.


The Helm charts are intended to simplify deployment and upgrades.
Configuring the Rook resources without Helm is also fully supported by creating the
[manifests](https://github.com/rook/rook/tree/master/deploy/examples)
directly.
