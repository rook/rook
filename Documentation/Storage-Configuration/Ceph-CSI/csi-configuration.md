---
title: CSI Configuration
---

CSI drivers are managed by the [ceph-csi-operator](https://github.com/ceph/ceph-csi-operator). This means CSI tuning is done
through CSI operator custom resources (`OperatorConfig` and `Driver`) and the ConfigMap `rook-csi-operator-image-set-configmap`.

!!! important
    The Rook ConfigMap `rook-ceph-operator-config` no longer applies CSI settings.

This document provides some example CSI settings that may need to be customized.

## OperatorConfig vs Driver

Per the [ceph-csi-operator design](https://github.com/ceph/ceph-csi-operator/blob/main/docs/design/operator.md),
`OperatorConfig` holds operator-wide defaults. Each `Driver` CR manages one driver instance and
allows per-driver customization. Driver `spec` fields take precedence over matching
`driverSpecDefaults` on `OperatorConfig`.

Many settings can be set on either resource. Use them as follows:

* **`OperatorConfig`** (`ceph-csi-operator-config`) — one per operator namespace. Set
    `spec.driverSpecDefaults` for defaults that apply to **all** CSI drivers (RBD, CephFS, NFS, and
    others), such as shared images, log level, controller replica count, or node plugin kubelet path.
* **`Driver`** — one CR per driver type (for example `rook-ceph.rbd.csi.ceph.com`). Use `spec` on
    a `Driver` to **override** those defaults for a single driver, or to set options that only apply
    to that driver (for example `deployCsiAddons` on RBD only, or `cephFsClientType` on CephFS).

## Default CSI Settings

The default CSI settings are applied by Rook in `deploy/examples/operator.yaml` (the section after the operator ConfigMap).

## CSI settings reference

For the complete list of ceph-csi-operator chart settings, see:
[Ceph-CSI Drivers chart configuration](https://github.com/ceph/ceph-csi-operator/blob/main/docs/helm-charts/drivers-chart.md#configuration).

## CSI-Addons sidecar

Previously: `CSI_ENABLE_CSIADDONS` in `rook-ceph-operator-config`.
Now set `deployCsiAddons` on `OperatorConfig` or `Driver`.

```yaml
# OperatorConfig (manifest)
apiVersion: csi.ceph.io/v1
kind: OperatorConfig
metadata:
  name: ceph-csi-operator-config
  namespace: rook-ceph
spec:
  driverSpecDefaults:
    deployCsiAddons: true
```

## Network Fencing

Network fencing requires:

1. Enable the [CSI-Addons controller](ceph-csi-drivers.md#csi-addons-controller).
2. Enable the CSI-Addons sidecar by setting `deployCsiAddons: true` on the RBD `Driver`.
3. Set `enableFencing` to `true` on the RBD `Driver`.

Here is an example of RBD `Driver`:

```yaml
# Driver CR (manifest)
apiVersion: csi.ceph.io/v1
kind: Driver
metadata:
  name: rook-ceph.rbd.csi.ceph.com
  namespace: rook-ceph
spec:
  deployCsiAddons: true
  enableFencing: true
```

## Custom container images

Previously: `CSI_*_IMAGE` keys in `rook-ceph-operator-config`. Now use the `ImageSet` ConfigMap referenced by `OperatorConfig`.

See `deploy/examples/operator.yaml`.

## Controller replicas and strategy

Previously: `CSI_PROVISIONER_REPLICAS` in `rook-ceph-operator-config`.
Now use `controllerPlugin.replicas` and `controllerPlugin.deploymentStrategy`.

```yaml
# OperatorConfig (manifest)
spec:
  driverSpecDefaults:
    controllerPlugin:
      replicas: 1
      deploymentStrategy:
        type: Recreate
```

## CephFS client type (kernel vs FUSE)

Previously: `CSI_FORCE_CEPHFS_KERNEL_CLIENT` in `rook-ceph-operator-config`.
Now use `cephFsClientType` on `OperatorConfig` or the CephFS `Driver`.

```yaml
# Driver-specific override (manifest)
apiVersion: csi.ceph.io/v1
kind: Driver
metadata:
  name: rook-ceph.cephfs.csi.ceph.com
  namespace: rook-ceph
spec:
  cephFsClientType: fuse
```

## Driver name prefix

Previously: `CSI_DRIVER_NAME_PREFIX` in `rook-ceph-operator-config`.
Now the `Driver` CR `metadata.name` is the provisioner name used in StorageClasses and
VolumeSnapshotClasses.

```yaml
apiVersion: csi.ceph.io/v1
kind: Driver
metadata:
  name: my-prefix.rbd.csi.ceph.com
  namespace: rook-ceph
spec:
  controllerPlugin: {}
  nodePlugin:
    updateStrategy:
      type: RollingUpdate
```

## Enable NFS CSI driver

Previously: NFS CSI deployment was reconciled by the Rook operator.
Now create the NFS `Driver` CR.

```console
kubectl apply -f deploy/examples/csi/nfs/driver.yaml
```

## Node plugin kubelet path and SELinux host mount

Previously: `CSI_KUBELET_DIR_PATH` and `CSI_PLUGIN_ENABLE_SELINUX_HOST_MOUNT` in
`rook-ceph-operator-config`.
Now use `nodePlugin.kubeletDirPath` and `nodePlugin.enableSeLinuxHostMount`.

```yaml
# OperatorConfig (manifest)
spec:
  driverSpecDefaults:
    nodePlugin:
      kubeletDirPath: /var/lib/kubelet
      enableSeLinuxHostMount: true
```
