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
* Preinstalled Rook Operator. See the [Helm Operator](helm-operator.md) topic to install.

## Installing

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration.
The [configuration](#configuration) section lists the parameters that can be configured during installation. It is
recommended that the rook operator be installed into the `rook-ceph` namespace. The clusters can be installed
into the same namespace as the operator or a separate namespace.

Rook currently publishes builds of this chart to the `release` and `master` channels.

**Before installing, review the values.yaml to confirm if the default settings need to be updated.**
- If the operator was installed in a namespace other than `rook-ceph`, the namespace
  must be set in the `operatorNamespace` variable.
- Set the desired settings in the `cephClusterSpec`. The [defaults](https://github.com/rook/rook/tree/{{ branchName }}/cluster/charts/rook-ceph-cluster/values.yaml)
  are only an example and not likely to apply to your cluster.
- The `monitoring` section should be removed from the `cephClusterSpec`, as it is specified separately in the helm settings.

### Release

The release channel is the most recent release of Rook that is considered stable for the community.

The example install assumes you have created a values-override.yaml.

```console
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster \
   --set operatorNamespace=rook-ceph rook-release/rook-ceph-cluster -f values-override.yaml
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
For the full list, see the [Cluster CRD](ceph-cluster-crd.md) topic.

### Existing Clusters

If you have an existing CephCluster CR that was created without the helm chart and you want the helm
chart to start managing the cluster:

1. Extract the `spec` section of your existing CephCluster CR and copy to the `cephClusterSpec`
   section in `values-override.yaml`.

2. Add the following annotations and label to your existing CephCluster CR:

```
  annotations:
    meta.helm.sh/release-name: rook-ceph-cluster
    meta.helm.sh/release-namespace: rook-ceph
  labels:
    app.kubernetes.io/managed-by: Helm
```

1. Run the `helm install` command in the [Installing section](#release) to create the chart.

2. In the future when updates to the cluster are needed, ensure the values-override.yaml always
   contains the desired CephCluster spec.

### Development Build

To deploy from a local build from your development environment:

```console
cd cluster/charts/rook-ceph-cluster
helm install --create-namespace --namespace rook-ceph rook-ceph-cluster -f values-override.yaml .
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
