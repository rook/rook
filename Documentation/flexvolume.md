---
title: Flex Volume Configuration
weight: 12
indent: true
---
# Flex Volume Configuration
Rook uses [FlexVolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#prerequisites) (the directory where flexvolume drivers are installed) is **read-only**.

This is the case for Kubernetes deployments on:
* [ContainerLinux](https://coreos.com/os/docs/latest/) (previously named CoreOS)
* [Rancher](http://rancher.com/)
* [Atomic](https://www.projectatomic.io/)

In these environments, the Kubelet needs to be told to use a different flexvolume plugin directory that is accessible and writeable (`rw`).
To do this, you will need to first add the `--volume-plugin-dir` flag to the Kubelet and then restart the Kubelet process.
These steps need to be carried out on **all nodes** in your cluster.

Please refer to all below sections that are applicable for you, they contain more information on your specific platform and/or Kubernetes version used.

```bash
--volume-plugin-dir=/var/lib/kubelet/volumeplugins
```
Restart Kubelet in order for this change to take effect.

## For Kubernetes >= 1.9.x
In Kubernetes >= `1.9.x`, you must provide the above set Flexvolume plugin directory when deploying the [rook-operator](/cluster/examples/kubernetes/ceph/operator.yaml) by setting the environment variable `FLEXVOLUME_DIR_PATH`. For example:
```yaml
- name: FLEXVOLUME_DIR_PATH
  value: "/var/lib/kubelet/volumeplugins"
```
(In the `operator.yaml` manifest replace `<PathToFlexVolumes>` with the path)


## For Rancher
Rancher provides an easy way to configure Kubelet. This flag can be provided to the Kubelet configuration template at deployment time or by using the `up to date` feature if Kubernetes is already deployed.

## For Tectonic
Follow [these instructions](tectonic.md) to configure the Flexvolume plugin for Rook on Tectonic during ContainerLinux node ignition file provisioning.
