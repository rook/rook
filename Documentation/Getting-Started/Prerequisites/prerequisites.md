---
title: Prerequisites
---

Rook can be installed on any existing Kubernetes cluster as long as it meets the minimum version
and Rook is granted the required privileges (see below for more information).

## Kubernetes Version

Kubernetes versions **v1.25** through **v1.30** are supported.

## CPU Architecture

Architectures supported are `amd64 / x86_64` and `arm64`.

## Ceph Prerequisites

To configure the Ceph storage cluster, at least one of these local storage types is required:

* Raw devices (no partitions or formatted filesystems)
* Raw partitions (no formatted filesystem)
* LVM Logical Volumes (no formatted filesystem)
* Persistent Volumes available from a storage class in `block` mode

Confirm whether the partitions or devices are formatted with filesystems with the following command:

```console
$ lsblk -f
NAME                  FSTYPE      LABEL UUID                                   MOUNTPOINT
vda
└─vda1                LVM2_member       >eSO50t-GkUV-YKTH-WsGq-hNJY-eKNf-3i07IB
  ├─ubuntu--vg-root   ext4              c2366f76-6e21-4f10-a8f3-6776212e2fe4   /
  └─ubuntu--vg-swap_1 swap              9492a3dc-ad75-47cd-9596-678e8cf17ff9   [SWAP]
vdb
```

If the `FSTYPE` field is not empty, there is a filesystem on top of the corresponding device. In this example, `vdb` is available to Rook, while `vda` and its partitions have a filesystem and are not available.

## LVM package

Ceph OSDs have a dependency on LVM in the following scenarios:

* If encryption is enabled (`encryptedDevice: "true"` in the cluster CR)
* A `metadata` device is specified
* `osdsPerDevice` is greater than 1

LVM is not required for OSDs in these scenarios:

* OSDs are created on raw devices or partitions
* OSDs are created on PVCs using the `storageClassDeviceSets`

If LVM is required, LVM needs to be available on the hosts where OSDs will be running.
Some Linux distributions do not ship with the `lvm2` package. This package is required on all storage nodes in the k8s cluster to run Ceph OSDs.
Without this package even though Rook will be able to successfully create the Ceph OSDs, when a node is rebooted the OSD pods
running on the restarted node will **fail to start**. Please install LVM using your Linux distribution's package manager. For example:

**CentOS**:

```console
sudo yum install -y lvm2
```

**Ubuntu**:

```console
sudo apt-get install -y lvm2
```

**RancherOS**:

* Since version [1.5.0](https://github.com/rancher/os/issues/2551) LVM is supported
* Logical volumes [will not be activated](https://github.com/rook/rook/issues/5027) during the boot process. You need to add an [runcmd command](https://rancher.com/docs/os/v1.x/en/installation/configuration/running-commands/) for that.

```yaml
runcmd:
- [ "vgchange", "-ay" ]
```

## Kernel

### RBD

Ceph requires a Linux kernel built with the RBD module. Many Linux distributions
have this module, but not all.
For example, the GKE Container-Optimised OS (COS) does not have RBD.

Test your Kubernetes nodes by running `modprobe rbd`.
If the rbd module is 'not found', rebuild the kernel to include the `rbd` module,
install a newer kernel, or choose a different Linux distribution.

Rook's default RBD configuration specifies only the `layering` feature, for
broad compatibility with older kernels. If your Kubernetes nodes run a 5.4
or later kernel, additional feature flags can be enabled in the
storage class. The `fast-diff` and `object-map` features are especially useful.

```yaml
imageFeatures: layering,fast-diff,object-map,deep-flatten,exclusive-lock
```

### CephFS

If creating RWX volumes from a Ceph shared file system (CephFS), the recommended minimum kernel version is **4.17**.
If the kernel version is less than 4.17, the requested PVC sizes will not be enforced. Storage quotas will only be
enforced on newer kernels.

## Distro Notes

Specific configurations for some distributions.

### NixOS

For NixOS, the kernel modules will be found in the non-standard path `/run/current-system/kernel-modules/lib/modules/`,
and they'll be symlinked inside the also non-standard path `/nix`.

Rook containers require read access to those locations to be able to load the required modules.
They have to be bind-mounted as volumes in the CephFS and RBD plugin pods.

If installing Rook with Helm, uncomment these example settings in `values.yaml`:

* `csi.csiCephFSPluginVolume`
* `csi.csiCephFSPluginVolumeMount`
* `csi.csiRBDPluginVolume`
* `csi.csiRBDPluginVolumeMount`

If deploying without Helm, add those same values to the settings in the `rook-ceph-operator-config`
ConfigMap found in operator.yaml:

* `CSI_CEPHFS_PLUGIN_VOLUME`
* `CSI_CEPHFS_PLUGIN_VOLUME_MOUNT`
* `CSI_RBD_PLUGIN_VOLUME`
* `CSI_RBD_PLUGIN_VOLUME_MOUNT`

If using containerd, remove `LimitNOFILE` from containerd service config to avoid issues like slow ceph commands or mons falling out of quorum.

```nix
systemd.services.containerd.serviceConfig = {
  LimitNOFILE = lib.mkForce null;
};
```
