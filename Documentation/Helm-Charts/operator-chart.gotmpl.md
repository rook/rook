---
title: Ceph Operator Helm Chart
---
{{ template "generatedDocsWarning" . }}

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Ceph clusters on Kubernetes.

## Introduction

This chart bootstraps a [rook-ceph-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

* Kubernetes 1.22+
* Helm 3.x

See the [Helm support matrix](https://helm.sh/docs/topics/version_skew/) for more details.

## Installing

The Ceph Operator helm chart will install the basic components necessary to create a storage platform for your Kubernetes cluster.

1. Install the Helm chart
1. [Create a Rook cluster](../Getting-Started/quickstart.md#create-a-ceph-cluster).

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation. It is recommended that the rook operator be installed into the `rook-ceph` namespace (you will install your clusters into separate namespaces).

Rook currently publishes builds of the Ceph operator to the `release` and `master` channels.

### **Release**

The release channel is the most recent release of Rook that is considered stable for the community.

```console
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph -f values.yaml
```

For example settings, see the next section or [values.yaml](https://github.com/rook/rook/tree/master/deploy/charts/rook-ceph/values.yaml)

## Configuration

The following table lists the configurable parameters of the rook-operator chart and their default values.

{{ template "chart.valuesTable" . }}

[^1]: `nodeAffinity` and `*NodeAffinity` options should have the format `"role=storage,rook; storage=ceph"` or `storage;role=rook-example` or `storage;` (_checks only for presence of key_)

### **Development Build**

To deploy from a local build from your development environment:

1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart:

```console
cd deploy/charts/rook-ceph
helm install --create-namespace --namespace rook-ceph rook-ceph .
```

## Uninstalling the Chart

To see the currently installed Rook chart:

```console
helm ls --namespace rook-ceph
```

To uninstall/delete the `rook-ceph` deployment:

```console
helm delete --namespace rook-ceph rook-ceph
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

After uninstalling you may want to clean up the CRDs as described on the [teardown documentation](../Storage-Configuration/ceph-teardown.md#removing-the-cluster-crd-finalizer).
