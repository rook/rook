---
title: Ceph Cluster
weight: 10200
indent: true
---

{% include_relative branch.liquid %}

# Ceph Cluster Helm Chart

Installs a [Ceph](https://ceph.io/) cluster on Rook using the [Helm](https://helm.sh) package manager.

## Prerequisites

* Kubernetes 1.13+
* Helm 3.x
* Preinstalled Rook Operator. See the [Helm Operator](https://rook.github.io/docs/rook/master/helm-operator.html) topic to install.

## Installing

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration.
The [configuration](#configuration) section lists the parameters that can be configured during installation. It is
recommended that the rook operator be installed into the `rook-ceph` namespace (you will install your clusters into
separate namespaces).

If the operator was installed in a non-default location, the namespace of the *Rook Operator* installed
must be set in the `operatorNamespace` variable.

Rook currently publishes builds of this chart to the `release` and `master` channels.

### Release

The release channel is the most recent release of Rook that is considered stable for the community.

```console
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph --set operatorNamespace=rook-ceph rook-release/rook-ceph-cluster
```

### Development Build

To deploy from a local build from your development environment:

1. Install the helm chart:

```console
cd cluster/charts/rook-ceph-cluster
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster .
```

## Uninstalling the Chart

To see the currently installed Rook chart:

```console
helm ls --namespace rook-ceph
```

To uninstall/delete the `rook-ceph-cluster` chart:

```console
helm delete --namespace rook-ceph rook-ceph-cluster
```

The command removes all the Kubernetes components associated with the chart and deletes the release. Removing the cluster
chart does not remove the Rook operator. In addition, all data on hosts in the Rook data directory
(`/var/lib/rook` by default) and on OSD raw devices is kept. To reuse disks, you will have to wipe them before recreating the cluster.

See the [teardown documentation](ceph-teardown.md) for more information.

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter               | Description                                                          | Default                 |
|-----------------------  |----------------------------------------------------------------------|-------------------------|
| `operatorNamespace`     | Namespace of the Rook Operator                                       | `rook-ceph`             |
| `configOverride`        | Cluster ceph.conf override                                           | <empty>                 |
| `toolbox.enabled`       | Enable Ceph debugging pod deployment. See [toolbox](ceph-toolbox.md) | `false`                 |
| `toolbox.tolerations`   | Toolbox tolerations                                                  | `[]`                    |
| `toolbox.affinity`      | Toolbox affinity                                                     | `{}`                    |
| `monitoring.enabled`    | Enable Prometheus integration, will also create necessary RBAC rules | `false`                 |
| `cephClusterSpec.*`     | Cluster configuration, see below                                     | See below               |

### Ceph Cluster Spec

The `CephCluster` CRD takes its spec from `cephClusterSpec.*`. This is not an exhaustive list of parameters.
For the full list, see the [Cluster CRD](https://rook.github.io/docs/rook/master/ceph-cluster-crd.html).

### Command Line

You can pass the settings with helm command line parameters. Specify each parameter using the
`--set key=value[,key=value]` argument to `helm install`.

### Settings File

Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while
installing the chart.

```console
helm install --namespace rook-ceph rook-ceph rook-release/rook-ceph-cluster -f values-override.yaml
```

For example settings, see [values.yaml](https://github.com/rook/rook/tree/{{ branchName }}/cluster/charts/rook-ceph-cluster/values.yaml)
