---
title: FlexVolume Configuration
weight: 12
indent: true
---
# FlexVolume Configuration
Rook uses [FlexVolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#prerequisites) (the directory where FlexVolume drivers are installed) is **read-only**.
This is the case for Kubernetes deployments on:

* [Atomic](https://www.projectatomic.io/)
* [ContainerLinux](https://coreos.com/os/docs/latest/) (previously named CoreOS)
* [OpenShift](https://www.openshift.com/)
* [Rancher](http://rancher.com/)

Especially in these environments, the kubelet needs to be told to use a different FlexVolume plugin directory that is accessible and read/write (`rw`).
These steps need to be carried out on **all nodes** in your cluster.

Please refer to the section that is applicable to your environment/platform, it contains more information on FlexVolume on your platform.

## Platform specific FlexVolume path
### Not a listed platform
If you are not using a platform that is listed above and the path `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/` is read/write, you don't need to configure anything.
That is because `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/` is the kubelet default FlexVolume path and Rook assumes the default FlexVolume path if not set differently.

If running `mkdir -p /usr/libexec/kubernetes/kubelet-plugins/volume/exec/` should give you an error about read-only filesystem, you need to use the [most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) and configure it on the Rook operator and kubelet.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Atomic
See the [OpenShift](#openshift) section.

### ContainerLinux
Use the [Most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) for the next steps.

The kubelet's systemD unit file can be located at: `/etc/systemd/system/kubelet.service`.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### OpenShift
To find out which FlexVolume directory path you need to set on the Rook operator, please look at the OpenShift docs of the version you are using, [latest OpenShift Flexvolume docs](https://docs.openshift.org/latest/install_config/persistent_storage/persistent_storage_flex_volume.html#flexvolume-installation) (they also contain the FlexVolume path for Atomic).

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Rancher
Rancher provides an easy way to configure kubelet. The FlexVolume flag will be shown later on in the [configuring the FlexVolume path](#configuring-the-flexvolume-path).
It can be provided to the kubelet configuration template at deployment time or by using the `up to date` feature if Kubernetes is already deployed.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Tectonic
Follow [these instructions](tectonic.md) to configure the Flexvolume plugin for Rook on Tectonic during ContainerLinux node ignition file provisioning.
If you want to use Rook with an already provisioned Tectonic cluster, please refer to the [ContainerLinux](#containerlinux) section.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Custom containerized kubelet
Use the [most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) for the next steps.

If your kubelet is running as a (Docker, rkt, etc) container you need to make sure that this directory from the host is reachable by the kubelet inside the container.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Configuring the FlexVolume path
If the environment specific section doesn't mention a FlexVolume path in this doc or external docs, please refer to the [most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) section, before continuing to [configuring the FlexVolume path](#configuring-the-flexvolume-path).

### Most common read/write FlexVolume path
The most commonly used read/write FlexVolume path on most systems is `/var/lib/kubelet/volumeplugins`.
This path is commonly used for FlexVolume because `/var/lib/kubelet` is read write on most systems.

### Configuring the Rook operator
You must provide the above found FlexVolume path when deploying the [rook-operator](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml) by setting the environment variable `FLEXVOLUME_DIR_PATH`.
For example:
```yaml
- name: FLEXVOLUME_DIR_PATH
  value: "/var/lib/kubelet/volumeplugins"
```

(In the `operator.yaml` manifest replace `<PathToFlexVolumes>` with the path or if you use helm set the `agent.flexVolumeDirPath` to the FlexVolume path)

### Configuring the Kubernetes kubelet
You need to add the flexvolume flag with the path to all nodes's kubelet in the Kubernetes cluster:
```
--volume-plugin-dir=PATH_TO_FLEXVOLUME
```
(Where the `PATH_TO_FLEXVOLUME` is the above found FlexVolume path)

The location where you can set the kubelet FlexVolume path (flag) depends on your platform.
Please refer to your platform documentation for that and/or the [platform specific FlexVolume path](#platform-specific-flexvolume-path) for information about that.

After adding the flag to kubelet, kubelet must be restarted for it to pick up the new flag.
