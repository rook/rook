---
title: Prerequisites
weight: 2010
indent: true
---

# Ceph Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, review the general [Rook Prerequisites](k8s-pre-reqs.md).

In order to configure the Ceph storage cluster, at least one of these local storage options are required:
- Raw devices (no partitions or formatted filesystems)
- Raw partitions (no formatted filesystem)
- PVs available from a storage class in `block` mode

## LVM package

Ceph OSDs have a dependency on LVM in the following scenarios:
- OSDs are created on raw devices or partitions
- If encryption is enabled (`encryptedDevice: true` in the cluster CR)
- A `metadata` device is specified

LVM is not required for OSDs in these scenarios:
- Creating OSDs on PVCs using the `storageClassDeviceSets`

If LVM is required for your scenario, LVM needs to be available on the hosts where OSDs will be running.
Some Linux distributions do not ship with the `lvm2` package. This package is required on all storage nodes in your k8s cluster to run Ceph OSDs.
Without this package even though Rook will be able to successfully create the Ceph OSDs, when a node is rebooted the OSD pods
running on the restarted node will **fail to start**. Please install LVM using your Linux distribution's package manager. For example:

CentOS:

```console
sudo yum install -y lvm2
```

Ubuntu:

```console
sudo apt-get install -y lvm2
```

RancherOS:

- Since version [1.5.0](https://github.com/rancher/os/issues/2551) LVM is supported
- Logical volumes [will not be activated](https://github.com/rook/rook/issues/5027) during the boot process. You need to add an [runcmd command](https://rancher.com/docs/os/v1.x/en/installation/configuration/running-commands/) for that.

```yaml
runcmd:
- [ vgchange, -ay ]
```

## Ceph Flexvolume Configuration

**NOTE** This configuration is only needed when using the FlexVolume driver (required for Kubernetes 1.12 or earlier). The Ceph-CSI RBD driver or the Ceph-CSI CephFS driver are recommended for Kubernetes 1.13 and newer, making FlexVolume configuration redundant.

If you want to configure volumes with the Flex driver instead of CSI, the Rook agent requires setup as a Flex volume plugin to manage the storage attachments in your cluster.
See the [Flex Volume Configuration](flexvolume.md) topic to configure your Kubernetes deployment to load the Rook volume plugin.

### Extra agent mounts

On certain distributions it may be necessary to mount additional directories into the agent container. That is what the environment variable `AGENT_MOUNTS` is for. Also see the documentation in [helm-operator](helm-operator.md) on the parameter `agent.mounts`. The format of the variable content should be `mountname1=/host/path1:/container/path1,mountname2=/host/path2:/container/path2`.

## Kernel

### RBD

Ceph requires a Linux kernel built with the RBD module. Many Linux distributions have this module, but not all distributions.
For example, the GKE Container-Optimised OS (COS) does not have RBD.

You can test your Kubernetes nodes by running `modprobe rbd`.
If it says 'not found', you may have to [rebuild your kernel](https://rook.io/docs/rook/master/common-issues.html#rook-agent-rbd-module-missing-error)
or choose a different Linux distribution.

### CephFS

If you will be creating volumes from a Ceph shared file system (CephFS), the recommended minimum kernel version is **4.17**.
If you have a kernel version less than 4.17, the requested PVC sizes will not be enforced. Storage quotas will only be
enforced on newer kernels.

## Kernel modules directory configuration

Normally, on Linux, kernel modules can be found in `/lib/modules`. However, there are some distributions that put them elsewhere. In that case the environment variable `LIB_MODULES_DIR_PATH` can be used to override the default. Also see the documentation in [helm-operator](helm-operator.md) on the parameter `agent.libModulesDirPath`. One notable distribution where this setting is useful would be [NixOS](https://nixos.org).
