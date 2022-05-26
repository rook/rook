---
title: Prerequisites
---

Rook can be installed on any existing Kubernetes cluster as long as it meets the minimum version
and Rook is granted the required privileges (see below for more information).

## Minimum Version

Kubernetes **v1.17** or higher is supported for the Ceph operator.

## CPU Architecture  

Architectures released are `amd64 / x86_64` and `arm64`.  

## Ceph Prerequisites

In order to configure the Ceph storage cluster, at least one of these local storage options are required:

* Raw devices (no partitions or formatted filesystems)
* Raw partitions (no formatted filesystem)
* PVs available from a storage class in `block` mode

You can confirm whether your partitions or devices are formatted with filesystems with the following command.

```console
$ lsblk -f
NAME                  FSTYPE      LABEL UUID                                   MOUNTPOINT
vda
└─vda1                LVM2_member       >eSO50t-GkUV-YKTH-WsGq-hNJY-eKNf-3i07IB
  ├─ubuntu--vg-root   ext4              c2366f76-6e21-4f10-a8f3-6776212e2fe4   /
  └─ubuntu--vg-swap_1 swap              9492a3dc-ad75-47cd-9596-678e8cf17ff9   [SWAP]
vdb
```

If the `FSTYPE` field is not empty, there is a filesystem on top of the corresponding device. In this example, you can use `vdb` for Ceph and can't use `vda` or its partitions.

## Admission Controller

Enabling the Rook admission controller is recommended to provide an additional level of validation that Rook is configured correctly with the custom resource (CR) settings. An admission controller intercepts requests to the Kubernetes API server prior to persistence of the object, but after the request is authenticated and authorized.

To deploy the Rook admission controllers, install the cert manager before Rook is installed:

```console
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.7.1/cert-manager.yaml
```

## LVM package

Ceph OSDs have a dependency on LVM in the following scenarios:

* OSDs are created on raw devices or partitions
* If encryption is enabled (`encryptedDevice: "true"` in the cluster CR)
* A `metadata` device is specified

LVM is not required for OSDs in these scenarios:

* Creating OSDs on PVCs using the `storageClassDeviceSets`

If LVM is required for your scenario, LVM needs to be available on the hosts where OSDs will be running.
Some Linux distributions do not ship with the `lvm2` package. This package is required on all storage nodes in your k8s cluster to run Ceph OSDs.
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

Ceph requires a Linux kernel built with the RBD module. Many Linux distributions have this module, but not all distributions.
For example, the GKE Container-Optimised OS (COS) does not have RBD.

You can test your Kubernetes nodes by running `modprobe rbd`.
If it says 'not found', you may have to rebuild your kernel and include at least the `rbd` module
or choose a different Linux distribution.

### CephFS

If you will be creating volumes from a Ceph shared file system (CephFS), the recommended minimum kernel version is **4.17**.
If you have a kernel version less than 4.17, the requested PVC sizes will not be enforced. Storage quotas will only be
enforced on newer kernels.
