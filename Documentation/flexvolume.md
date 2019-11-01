---
title: FlexVolume Configuration
weight: 1200
indent: true
---

# FlexVolume Configuration

Rook uses [FlexVolume](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-storage/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-storage/flexvolume.md#prerequisites) (the directory where FlexVolume drivers are installed) is **read-only**.
This is the case for Kubernetes deployments on:

* [Atomic](https://www.projectatomic.io/)
* [ContainerLinux](https://coreos.com/os/docs/latest/) (previously named CoreOS)
* [OpenShift](https://www.openshift.com/)
* [Rancher](http://rancher.com/)
* [Google Kubernetes Engine (GKE)](https://cloud.google.com/kubernetes-engine/)
* [Azure AKS](https://docs.microsoft.com/en-us/azure/aks/)

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

See the [OpenShift](#openshift) section, unless running with OpenStack Magnum, then see [OpenStack Magnum](#openstack-magnum) section.

### ContainerLinux

Use the [Most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) for the next steps.

The kubelet's systemD unit file can be located at: `/etc/systemd/system/kubelet.service`.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Kubespray

Kubespray uses a non-standard [FlexVolume plugin directory](https://github.com/kubernetes-sigs/kubespray/blob/master/roles/kubernetes/node/defaults/main.yml#L55): `/var/lib/kubelet/volume-plugins`.
The Kubespray configured kubelet is already configured to use that directory.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### OpenShift

To find out which FlexVolume directory path you need to set on the Rook operator, please look at the OpenShift docs of the version you are using, [latest OpenShift Flexvolume docs](https://docs.openshift.org/latest/install_config/persistent_storage/persistent_storage_flex_volume.html#flexvolume-installation) (they also contain the FlexVolume path for Atomic).

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Rancher

Rancher provides an easy way to configure kubelet. The FlexVolume flag will be shown later on in the [configuring the FlexVolume path](#configuring-the-flexvolume-path).
It can be provided to the kubelet configuration template at deployment time or by using the `up to date` feature if Kubernetes is already deployed.

Rancher deploys kubelet as a docker container, you need to mount the host's flexvolume path into the kubelet image as a volume,
this can be done in the `extra_binds` section of the kubelet cluster config.

Configure the Rancher deployed kubelet by updating the `cluster.yml` file kubelet section:

```yaml
services:
  kubelet:
    extra_args:
      volume-plugin-dir: /usr/libexec/kubernetes/kubelet-plugins/volume/exec
    extra_binds:
      - /usr/libexec/kubernetes/kubelet-plugins/volume/exec:/usr/libexec/kubernetes/kubelet-plugins/volume/exec
```

If you're using [rke](https://github.com/rancher/rke), run `rke up`, this will update and restart your kubernetes cluster system components, in this case the kubelet docker instance(s)
will get restarted with the new volume bind and volume plugin dir flag.

The default FlexVolume path for Rancher is `/usr/libexec/kubernetes/kubelet-plugins/volume/exec` which is also the default
FlexVolume path for the Rook operator.

If the default path as above is used no further configuration is required, otherwise if a different path is used
the Rook operator will need to be reconfigured, to do this continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Google Kubernetes Engine (GKE)

Google's Kubernetes Engine uses a non-standard FlexVolume plugin directory: `/home/kubernetes/flexvolume`
The kubelet on GKE is already configured to use that directory.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Azure AKS

AKS uses a non-standard FlexVolume plugin directory: `/etc/kubernetes/volumeplugins`
The kubelet on AKS is already configured to use that directory.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### Tectonic

Follow [these instructions](tectonic.md) to configure the Flexvolume plugin for Rook on Tectonic during ContainerLinux node ignition file provisioning.
If you want to use Rook with an already provisioned Tectonic cluster, please refer to the [ContainerLinux](#containerlinux) section.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### OpenStack Magnum

OpenStack Magnum is using Atomic, which uses a non-standard FlexVolume plugin directory at:  `/var/lib/kubelet/volumeplugins`
The kubelet in OpenStack Magnum is already configured to use that directory.
You will need to use this value when [configuring the Rook operator](#configuring-the-rook-operator)

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

**Example**:

```yaml
spec:
  template:
    spec:
      containers:
[...]
      - name: rook-ceph-operator
        env:
[...]
        - name: FLEXVOLUME_DIR_PATH
          value: "/var/lib/kubelet/volumeplugins"
[...]
```

(In the `operator.yaml` manifest replace `<PathToFlexVolumes>` with the path or if you use helm set the `agent.flexVolumeDirPath` to the FlexVolume path)

### Configuring the Kubernetes kubelet

You need to add the flexvolume flag with the path to all nodes's kubelet in the Kubernetes cluster:

```console
--volume-plugin-dir=PATH_TO_FLEXVOLUME
```

(Where the `PATH_TO_FLEXVOLUME` is the above found FlexVolume path)

The location where you can set the kubelet FlexVolume path (flag) depends on your platform.
Please refer to your platform documentation for that and/or the [platform specific FlexVolume path](#platform-specific-flexvolume-path) for information about that.

After adding the flag to kubelet, kubelet must be restarted for it to pick up the new flag.
