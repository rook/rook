---
title: Flex Volume Configuration
weight: 12
indent: true
---

# Flex Volume Configuration

Rook uses [FlexVolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#prerequisites) (the directory where flexvolume drivers are installed) is **read-only**.

This is the case for Kubernetes deployments on CoreOS and Rancher.
In these environments, the Kubelet needs to be told to use a different flexvolume plugin directory that is accessible and writeable.
To do this, you will need to first add the `--volume-plugin-dir` flag to the Kubelet and then restart the Kubelet process. 
These steps need to be carried out on **all nodes** in your cluster.

## CoreOS Container Linux

In CoreOS, our recommendation is to specify the flag as shown below:

```bash
--volume-plugin-dir=/var/lib/kubelet/volumeplugins
```

Restart Kubelet in order for this change to take effect.

In Kubernetes 1.9.x, you must provide this directory when deploying the [rook-operator](/cluster/examples/kubernetes/rook-operator.yaml) by setting the environment variable `FLEXVOLUME_DIR_PATH`. For example:
```yaml
- name: FLEXVOLUME_DIR_PATH
  value: "/var/lib/kubelet/volumeplugins"
```

## Rancher

Rancher provides an easy way to configure Kubelet. This flag can be provided to the Kubelet configuration template at deployment time or by using the `up to date` feature if Kubernetes is already deployed.

To configure Flexvolume in Rancher, specify this Kubelet flag as shown below:

```bash
--volume-plugin-dir=/var/lib/kubelet/volumeplugins
```

Restart Kubelet in order for this change to take effect.

In Kubernetes 1.9.x, you must provide this directory when deploying the [rook-operator](/cluster/examples/kubernetes/rook-operator.yaml) by setting the environment variable `FLEXVOLUME_DIR_PATH`. For example:
```yaml
- name: FLEXVOLUME_DIR_PATH
  value: "/var/lib/kubelet/volumeplugins"
```

## Tectonic

Follow [these instructions](tectonic.md) to configure Rook on Tectonic.
