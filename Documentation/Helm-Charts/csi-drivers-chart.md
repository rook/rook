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

**IMPORTANT**

1. Install this chart with the recommended values.yaml. The drivers will fail if only configured with the chart defaults.
2. If installing in another namespace, replace all instances of `rook-ceph` in the values.yaml with the required namespace.

```console
helm repo add ceph-csi-operator https://ceph.github.io/ceph-csi-operator
helm install ceph-csi-drivers --namespace rook-ceph ceph-csi-operator/ceph-csi-drivers \
  -f https://raw.githubusercontent.com/rook/rook/master/deploy/charts/ceph-csi-drivers/values.yaml
```

## Custom settings

Below are some examples of common settings that may need to be customized in the CSI drivers chart. Create a values file with the desired settings and install with `-f values.yaml`.

### CSI-Addons sidecar

```yaml
operatorConfig:
  driverSpecDefaults:
    deployCsiAddons: true
```

See: [CSI-Addons sidecar](../Storage-Configuration/Ceph-CSI/csi-configuration.md#csi-addons-sidecar)

### Network Fencing

The example below is scoped to the RBD driver only.

```yaml
drivers:
  rbd:
    deployCsiAddons: true
    enableFencing: true
```

!!! important
    The Network Fencing feature requires the CSI-Addons controller to be deployed separately for auto-unfencing. See
    [Deploying the controller](../Storage-Configuration/Ceph-CSI/ceph-csi-drivers.md#deploying-the-controller).

See: [Network Fencing](../Storage-Configuration/Ceph-CSI/csi-configuration.md#network-fencing)

### Controller plugin replicas

```yaml
operatorConfig:
  driverSpecDefaults:
    controllerPlugin:
      replicas: 2
```

See: [Controller replicas and strategy](../Storage-Configuration/Ceph-CSI/csi-configuration.md#controller-replicas-and-strategy)

### CephFS client type

```yaml
drivers:
  cephfs:
    cephFsClientType: fuse
```

See: [CephFS client type (kernel vs FUSE)](../Storage-Configuration/Ceph-CSI/csi-configuration.md#cephfs-client-type-kernel-vs-fuse)

### RBD driver name prefix

```yaml
drivers:
  rbd:
    name: rook-ceph.rbd.csi.ceph.com
```

**Note** The prefix `rook-ceph` should always be the rook operator namespace.

See: [Driver name / provisioner prefix](../Storage-Configuration/Ceph-CSI/csi-configuration.md#driver-name-prefix)

### NFS CSI driver

```yaml
drivers:
  nfs:
    enabled: true
    name: rook-ceph.nfs.csi.ceph.com
```

See: [Enable NFS CSI driver](../Storage-Configuration/Ceph-CSI/csi-configuration.md#enable-nfs-csi-driver)

### kubelet path

```yaml
operatorConfig:
  driverSpecDefaults:
    nodePlugin:
      kubeletDirPath: /var/lib/kubelet
      enableSeLinuxHostMount: true
```

See: [Node plugin kubelet path and SELinux host mount](../Storage-Configuration/Ceph-CSI/csi-configuration.md#node-plugin-kubelet-path-and-selinux-host-mount)

### Custom CSI images

Custom CSI images are configured from the [rook-ceph](operator-chart.md#configuration) chart.
See the default images in the `rook-ceph` chart [values](https://github.com/rook/rook/blob/release-1.20/deploy/charts/rook-ceph/values.yaml#L97-L137)
