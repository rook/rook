---
title: Nfsh Operator
weight: 10100
indent: true
---

# Nfs Operator Helm Chart

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Nfs on Kubernetes.

## Introduction

This chart bootstraps a [rook-nfs-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.10+
- A Kubernetes cluster is necessary to run the Rook NFS operator. To make sure you have a Kubernetes cluster that is ready for Rook, you can follow these [instructions](https://rook.io/docs/rook/master/k8s-pre-reqs.html).
- NFS client packages must be installed on all nodes where Kubernetes might run pods with NFS mounted. Install nfs-utils on CentOS nodes or nfs-common on Ubuntu nodes.

### RBAC

If role-based access control (RBAC) is enabled in your cluster, you may need to give Tiller (the server-side component of Helm) additional permissions. **If RBAC is not enabled, be sure to set `rbacEnable` to `false` when installing the chart.**

```console
# Create a ServiceAccount for Tiller in the `kube-system` namespace
kubectl --namespace kube-system create sa tiller

# Create a ClusterRoleBinding for Tiller
kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller

# Patch Tiller's Deployment to use the new ServiceAccount
kubectl --namespace kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
```

## Installing

```console
helm repo add rook-release https://charts.rook.io/release
helm install --namespace rook-nfs rook-release/rook-nfs
```

### Development Build
To deploy from a local build from your development environment:
1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart
```console
cd cluster/charts/rook-nfs
helm install --namespace rook-nfs --name rook-nfs .
```

## Uninstalling the Chart

To uninstall/delete the `rook-nfs` deployment:

```console
$ helm delete --purge rook-nfs
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter                    | Description                                                                                             | Default                                                |
| ---------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `image.repository`           | Image                                                                                                   | `rook/nfs`                                             |
| `image.tag`                  | Image tag                                                                                               | `master`                                               |
| `image.pullPolicy`           | Image pull policy                                                                                       | `IfNotPresent`                                         |
| `rbacEnable`                 | If true, create & use RBAC resources                                                                    | `true`                                                 |
| `annotations`                | Pod annotations                                                                                         | `{}`                                                   |
| `logLevel`                   | Global log level                                                                                        | `INFO`                                                 |
| `nodeSelector`               | Kubernetes `nodeSelector` to add to the Deployment.                                                     | <none>                                                 |


### Command Line
You can pass the settings with helm command line parameters. Specify each parameter using the
`--set key=value[,key=value]` argument to `helm install`. For example, the following command will install rook where RBAC is not enabled.

```console
$ helm install --namespace rook-nfs --name rook-nfs rook-release/rook-nfs --set rbacEnable=false
```

### Settings File
Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while installing the chart.

```console
$ helm install --namespace rook-nfs --name rook-nfs rook-release/rook-nfs -f values.yaml
```

Here are the sample settings to get you started.

```yaml
image:
  prefix: rook
  repository: rook/nfs
  tag: master
  pullPolicy: IfNotPresent

hyperkube:
  repository: k8s.gcr.io/hyperkube
  tag: v1.7.12
  pullPolicy: IfNotPresent

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 128Mi

provisioner:
  name: "rook.io/nfs-provisioner"
```
