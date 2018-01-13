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

1. Create a ServiceAccount for Tiller in the `kube-system` namespace
  ```console
  $ kubectl -n kube-system create sa tiller
  ```

2. Create a ClusterRoleBinding for Tiller
  ```console
  $ kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller
  ```

3. Patch Tiller's Deployment to use the new ServiceAccount
  ```console
  $ kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
  ```

## Installing

To install the chart from out published registry, run the following:

```console
$ helm repo add rook-<channel> https://charts.rook.io/<channel>
$ helm install rook-<channel>/rook
```

Be sure to replace `<channel>` with `alpha` or `master` (in the future `beta` and `stable` when available), for example:

```console
$ helm repo add rook-alpha https://charts.rook.io/alpha
$ helm install rook-alpha/rook
```

The command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

Alternatively, to deploy from a local checkout of the rook codebase:

```console
$ cd cluster/charts/rook
$ helm install --name rook --namespace rook .
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
| `image.tag`        | Image tag                            | `master`             |
| `image.pullPolicy` | Image pull policy                    | `IfNotPresent`       |
| `rbacEnable`       | If true, create & use RBAC resources | `true`               |
| `resources`        | Pod resource requests & limits       | `{}`                 |
| `logLevel`        | Global log level        | `INFO`                 |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example to disable RBAC,

```console
$ helm install --name rook rook-alpha/rook --set rbacEnable=false
```

Alternatively, a yaml file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm install --name rook rook-alpha/rook -f values.yaml
```

### Defaults

Here are the sample settings to get you started.

```yaml
image:
  prefix: rook
  repository: rook/rook
  tag: master
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
