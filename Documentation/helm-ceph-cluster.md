---
title: Ceph Cluster
weight: 10200
indent: true
---

{% include_relative branch.liquid %}

# Ceph Cluster Helm Chart

Install a Rook Ceph cluster using the [Helm](https://helm.sh) package manager.

## Experimental

This chart is released to the community for testing but is considered in experimental. It is currently only available on the `master` channel.
Once released (targeted for v1.7), the repository should be updated to the `release` channel at that time for the stable branch.

## Prerequisites

* Kubernetes 1.13+
* Helm 3.x
* Preinstalled Rook Operator. See the [Helm Operator](helm-operator.md) topic to install.

## Installing

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration.
The [configuration](#configuration) section lists the parameters that can be configured during installation. It is
recommended that the rook operator be installed into the `rook-ceph` namespace. The clusters can be installed
into the same namespace as the operator or a separate namespace.

If the operator was installed in a namespace other than `rook-ceph`, the namespace
must be set in the `operatorNamespace` variable.

Rook currently publishes builds of this chart to the `master` channel.

### Master Channel

The master channel is the most recent release of Rook that includes experimental features.

**Before installing, review the values.yaml to confirm if the default settings need to be updated.
The [defaults](https://github.com/rook/rook/tree/master/cluster/charts/rook-ceph-cluster/values.yaml)
are only an example and not likely to apply to your cluster.**

The example install assumes you have created a values-override.yaml.

```console
helm repo add rook-master https://charts.rook.io/master
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster \
    --set operatorNamespace=rook-ceph rook-master/rook-ceph-cluster -f values-override.yaml
```

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter             | Description                                                          | Default     |
| --------------------- | -------------------------------------------------------------------- | ----------- |
| `operatorNamespace`   | Namespace of the Rook Operator                                       | `rook-ceph` |
| `configOverride`      | Cluster ceph.conf override                                           | <empty>     |
| `toolbox.enabled`     | Enable Ceph debugging pod deployment. See [toolbox](ceph-toolbox.md) | `false`     |
| `toolbox.tolerations` | Toolbox tolerations                                                  | `[]`        |
| `toolbox.affinity`    | Toolbox affinity                                                     | `{}`        |
| `monitoring.enabled`  | Enable Prometheus integration, will also create necessary RBAC rules | `false`     |
| `cephClusterSpec.*`   | Cluster configuration, see below                                     | See below   |

### Ceph Cluster Spec

The `CephCluster` CRD takes its spec from `cephClusterSpec.*`. This is not an exhaustive list of parameters.
For the full list, see the [Cluster CRD](https://rook.github.io/docs/rook/master/ceph-cluster-crd.html).

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
