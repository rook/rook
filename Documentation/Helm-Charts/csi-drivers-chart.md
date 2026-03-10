---
title: Ceph-CSI Driver Helm Chart
---

To configure the Ceph-CSI drivers, Rook requires the installation of the [Ceph-CSI Driver chart](https://github.com/ceph/ceph-csi-operator/blob/main/docs/helm-charts/drivers-chart.md). This chart configures the CSI drivers to provision and mount volumes to make available the Ceph storage to your applications.

## Prerequisites

* The `rook-ceph` chart must be installed before the Ceph-CSI drivers chart, to install the required Ceph-CSI operator and CRDs.

## Installing

The Ceph-CSI drivers Helm chart installs the resources needed for [ceph-csi](https://github.com/ceph/ceph-csi) to run under the ceph-csi-operator.

The `helm install` command deploys the drivers in the default configuration from the chart. For more configuration options, see the [Ceph-CSI Drivers Configuration](https://github.com/ceph/ceph-csi-operator/blob/main/docs/helm-charts/drivers-chart.md#configuration).

Ceph-CSI publishes the drivers chart from the `ceph-csi-operator` Helm repository.

```console
helm repo add ceph-csi-operator https://ceph.github.io/ceph-csi-operator
helm install ceph-csi-drivers --namespace rook-ceph ceph-csi-operator/ceph-csi-drivers
```
