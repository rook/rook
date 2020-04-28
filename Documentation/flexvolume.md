---
title: FlexVolume Configuration
weight: 1200
indent: true
---

# Ceph FlexVolume Configuration

FlexVolume is not enabled by default since Rook v1.1. This documentation applies only if you have enabled FlexVolume.

If enabled, Rook uses [FlexVolume](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-storage/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-storage/flexvolume.md#prerequisites) (the directory where FlexVolume drivers are installed) is **read-only**.

Some Kubernetes deployments require you to configure kubelet with a FlexVolume plugin directory that is accessible and read/write (`rw`). These steps need to be carried out on **all nodes** in your cluster. Rook needs to be told where this directory is in order for the volume plugin to work.

Platform-specific instructions for the following Kubernetes deployment platforms are linked below

* [Default FlexVolume path](#default-flexvolume-path)
* [Atomic](#atomic)
* [Azure AKS](#azure-aks)
* [ContainerLinux](#containerlinux)
* [Google Kubernetes Engine (GKE)](#google-kubernetes-engine-gke)
* [Kubespray](#kubespray)
* [OpenShift](#openshift)
* [OpenStack Magnum](#openstack-magnum)
* [Rancher](#rancher)
* [Tectonic](#tectonic)
* [Custom containerized kubelet](#custom-containerized-kubelet)
* [Configuring the FlexVolume path](#configuring-the-flexvolume-path)

## Default FlexVolume path

If you are not using a platform that is listed above and the path `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/` is read/write, you don't need to configure anything.

That is because `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/` is the kubelet default FlexVolume path and Rook assumes the default FlexVolume path if not set differently.

If running `mkdir -p /usr/libexec/kubernetes/kubelet-plugins/volume/exec/` should give you an error about read-only filesystem, you need to use [another read/write FlexVolume path](#other-common-readwrite-flexvolume-paths) and configure it on the Rook operator and kubelet.

These are the other commonly used paths:

* `/var/lib/kubelet/volumeplugins`
* `/var/lib/kubelet/volume-plugins`

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Atomic

See the [OpenShift](#openshift) section, unless running with OpenStack Magnum, then see [OpenStack Magnum](#openstack-magnum) section.

## Azure AKS

AKS uses a non-standard FlexVolume plugin directory: `/etc/kubernetes/volumeplugins`
The kubelet on AKS is already configured to use that directory.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## ContainerLinux

Use the [Most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) for the next steps.

The kubelet's systemD unit file can be located at: `/etc/systemd/system/kubelet.service`.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Google Kubernetes Engine (GKE)

Google's Kubernetes Engine uses a non-standard FlexVolume plugin directory: `/home/kubernetes/flexvolume`
The kubelet on GKE is already configured to use that directory.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Kubespray

### Prior to v2.11.0

Kubespray uses the [kubelet_flexvolumes_plugins_dir](https://github.com/kubernetes-sigs/kubespray/blob/v2.11.0/inventory/sample/group_vars/k8s-cluster/k8s-cluster.yml#L206) variable to define where it sets the plugin directory.

Kubespray prior to the v2.11.0 release [used a non-standard FlexVolume plugin directory](https://github.com/kubernetes-sigs/kubespray/blob/f47a66622743aa31970cebeca7968a0939cb700d/roles/kubernetes/node/defaults/main.yml#L53): `/var/lib/kubelet/volume-plugins`.
The Kubespray configured kubelet is already configured to use that directory.

If you are using kubespray v2.10.x or older, continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

### As of v2.11.0 and newer

Kubespray v2.11.0 included https://github.com/kubernetes-sigs/kubespray/pull/4752 which sets the same plugin directory assumed by rook by default: `/usr/libexec/kubernetes/kubelet-plugins/volume/exec`.

No special configuration of the directory is needed in Rook unless:

* Kubespray is deployed onto a platform where the default path is not writable, or
* you have explicitly defined a custom path in the [kubelet_flexvolumes_plugins_dir](https://github.com/kubernetes-sigs/kubespray/blob/v2.11.0/inventory/sample/group_vars/k8s-cluster/k8s-cluster.yml#L206) variable

If you have not defined one, and the default path is not writable, the [alternate configuration](https://github.com/kubernetes-sigs/kubespray/blob/v2.11.0/roles/kubernetes/preinstall/tasks/0040-set_facts.yml#L189) is `/var/lib/kubelet/volumeplugins`

If needed, continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## OpenShift

To find out which FlexVolume directory path you need to set on the Rook operator, please look at the OpenShift docs of the version you are using, [latest OpenShift Flexvolume docs](https://docs.openshift.org/latest/install_config/persistent_storage/persistent_storage_flex_volume.html#flexvolume-installation) (they also contain the FlexVolume path for Atomic).

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## OpenStack Magnum

OpenStack Magnum is using Atomic, which uses a non-standard FlexVolume plugin directory at:  `/var/lib/kubelet/volumeplugins`
The kubelet in OpenStack Magnum is already configured to use that directory.
You will need to use this value when [configuring the Rook operator](#configuring-the-rook-operator)

## Rancher

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

## Tectonic

Follow [these instructions](tectonic.md) to configure the Flexvolume plugin for Rook on Tectonic during ContainerLinux node ignition file provisioning.
If you want to use Rook with an already provisioned Tectonic cluster, please refer to the [ContainerLinux](#containerlinux) section.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Custom containerized kubelet

Use the [most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) for the next steps.

If your kubelet is running as a (Docker, rkt, etc) container you need to make sure that this directory from the host is reachable by the kubelet inside the container.

Continue with [configuring the FlexVolume path](#configuring-the-flexvolume-path) to configure Rook to use the FlexVolume path.

## Configuring the FlexVolume path

If the environment specific section doesn't mention a FlexVolume path in this doc or external docs, please refer to the [most common read/write FlexVolume path](#most-common-readwrite-flexvolume-path) section, before continuing to [configuring the FlexVolume path](#configuring-the-flexvolume-path).

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
