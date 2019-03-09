---
title: Cockroachdb Operator
weight: 10100
indent: true
---

# Cockroachdb Operator Helm Chart

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Cockroachdb clusters on Kubernetes.

## Introduction

This chart bootstraps a [rook-cockroachdb-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.10+

### RBAC

If role-based access control (RBAC) is enabled in your cluster, you may need to give Tiller (the server-side component of Helm) additional permissions.

```console
# Create a ServiceAccount for Tiller in the `kube-system` namespace
kubectl --namespace kube-system create sa tiller

# Create a ClusterRoleBinding for Tiller
kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller

# Patch Tiller's Deployment to use the new ServiceAccount
kubectl --namespace kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
```

## Installing

The Cockroachdb Operator helm chart will install the basic components necessary to create a storage platform for your Kubernetes cluster.
After the helm chart is installed, you will need to [create a Rook cluster](cockroachdb.md).

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation. It is recommended that the rook operator be installed into the `rook-cockroachdb-system` namespace (you will install your clusters into separate namespaces).

Rook currently publishes builds of the Cockroachdb operator to the `stable` and `master` channels.

### Stable
The stable channel is the most recent release of Rook that is considered stable for the community, starting with the v0.9 release.

```console
helm repo add rook-stable https://charts.rook.io/stable
helm install --namespace rook-cockroachdb-system rook-stable/rook-cockroachdb
```

### Master
The master channel includes the latest commits, with all automated tests green. Historically it has been very stable, though there is no guarantee.

To install the helm chart from master, you will need to pass the specific version returned by the `search` command.
```console
helm repo add cockroachdb-master https://charts.rook.io/master
helm search rook-cockroachdb
helm install --namespace rook-cockroachdb-system rook-master/rook-cockroachdb --version <version>
```

For example:
```
helm install --namespace rook-cockroachdb-system rook-master/rook-cockroachdb --version v0.7.0-278.gcbd9726
```

### Development Build
To deploy from a local build from your development environment:
1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart
```console
cd cluster/charts/rook-cockroachdb
helm install --namespace rook-cockroachdb-system --name rook-cockroachdb .
```

## Uninstalling the Chart

To uninstall/delete the `rook-cockroachdb` deployment:

```console
$ helm delete --purge rook-cockroachdb
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter                    | Description                                                                                             | Default                                                |
| ---------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `image.repository`           | Image                                                                                                   | `rook/cockroachdb`                                            |
| `image.tag`                  | Image tag                                                                                               | `master`                                               |
| `image.pullPolicy`           | Image pull policy                                                                                       | `IfNotPresent`                                         |
| `annotations`                | Pod annotations                                                                                         | `{}`                                                   |
| `nodeSelector`               | Kubernetes `nodeSelector` to add to the Deployment.                                                     | <none>                                                 |                                                 | `300s`                                                 |

### Command Line
You can pass the settings with helm command line parameters. Specify each parameter using the
`--set key=value[,key=value]` argument to `helm install`. For example, the following command will install rook to set image.pullPolicy.

```console
$ helm install --namespace rook-cockroachdb-system --name rook-cockroachdb rook-stable/rook-cockroachdb --set image.pullPolicy=IfNotPresent
```

### Settings File
Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while installing the chart.

```console
$ helm install --namespace rook-cockroachdb-system --name rook-cockroachdb rook-stable/rook-cockroachdb -f values.yaml
```

Here are the sample settings to get you started.

```yaml
image:
  repository: rook/cockroachdb
  tag: master
  pullPolicy: IfNotPresent

nodeSelector: {}

tolerations: []
```
