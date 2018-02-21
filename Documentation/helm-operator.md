---
title: Operator
weight: 51
indent: true
---

# Operator Helm Chart

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Rook clusters on Kubernetes.

## Introduction

This chart bootstraps a [rook-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.6+

### RBAC

If role-based access control (RBAC) is enabled in your cluster, you may need to give Tiller (the server-side component of Helm) additional permissions. **If RBAC is not enabled, be sure to set `rbacEnable` to `false` when installing the chart.**

```console
# Create a ServiceAccount for Tiller in the `kube-system` namespace
kubectl -n kube-system create sa tiller

# Create a ClusterRoleBinding for Tiller
kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller

# Patch Tiller's Deployment to use the new ServiceAccount
kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
```

## Installing

The Rook Operator helm chart will install the basic components necessary to create a storage platform for your Kubernetes cluster. 
After the helm chart is installed, you will need to [create a Rook cluster](quickstart.md#create-a-rook-cluster).

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

Rook currently publishes builds to the `alpha` and `master` channels. In the future, `beta` and `stable` will also be available.

### Alpha
The alpha channel is the most recent release of Rook that is considered ready for testing by the community. 
```console
helm repo add rook-alpha https://charts.rook.io/alpha
helm install rook-alpha/rook
```

### Master
The master channel includes the latest commits, with all automated tests green. Historically it has been very stable, though there is no guarantee.

To install the helm chart from master, you will need to pass the specific version returned by the `search` command.
```console
helm repo add rook-master https://charts.rook.io/master
helm search rook
helm install rook-master/rook --version <version>
```

For example:
```
helm install rook-master/rook --version v0.6.0-156.gef983d6
```

### Development Build
To deploy from a local build from your development environment:
1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart
```console
cd cluster/charts/rook
helm install --name rook --namespace rook-system .
```

## Uninstalling the Chart

To uninstall/delete the `rook` deployment:

```console
$ helm delete --purge rook
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter          | Description                          | Default              |
|--------------------|--------------------------------------|----------------------|
| `image.repository` | Image                                | `rook/rook`          |
| `image.tag`        | Image tag                            | `v0.7.0`             |
| `image.pullPolicy` | Image pull policy                    | `IfNotPresent`       |
| `rbacEnable`       | If true, create & use RBAC resources | `true`               |
| `resources`        | Pod resource requests & limits       | `{}`                 |
| `logLevel`         | Global log level        | `INFO`                 |
| `agent.flexVolumeDirPath` | Path where the Rook agent discovers the flex volume plugins | `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/` |
| `agent.toleration`        | Toleration for the agent pods | <none> |
| `agent.tolerationKey`     | The specific key of the taint to tolerate | <none> |
| `mon.healthCheckInterval` | The frequency for the operator to check the mon health | `45s` |
| `mon.monOutTimeout`       | The time to wait before failing over an unhealthy mon | `300s` |


### Command Line
You can pass the settings with helm command line parameters. Specify each parameter using the 
`--set key=value[,key=value]` argument to `helm install`. For example, the following command will install rook where RBAC is not enabled.

```console
$ helm install --name rook rook-alpha/rook --set rbacEnable=false
```

### Settings File
Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while installing the chart.

```console
$ helm install --name rook rook-alpha/rook -f values.yaml
```

Here are the sample settings to get you started.

```yaml
image:
  prefix: rook
  repository: rook/rook
  tag: v0.7.0
  pullPolicy: IfNotPresent

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 128Mi

rbacEnable: true
```
