---
title: Rook Upgrades
---

This guide will walk through the steps to upgrade the software in a Rook cluster from one
version to the next. This guide focuses on updating the Rook version for the management layer,
while the [Ceph upgrade](ceph-upgrade.md) guide focuses on updating the data layer.

Upgrades for both the operator and for Ceph are entirely automated except where Rook's
permissions need to be explicitly updated by an admin or when incompatibilities need to be addressed
manually due to customizations.

We welcome feedback and opening issues!

## Supported Versions

This guide is for upgrading from **Rook v1.20.x to Rook v1.21.x**.

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

* [Upgrade 1.19 to 1.20](https://rook.io/docs/rook/v1.20/Upgrade/rook-upgrade/)
* [Upgrade 1.18 to 1.19](https://rook.io/docs/rook/v1.19/Upgrade/rook-upgrade/)
* [Upgrade 1.17 to 1.18](https://rook.io/docs/rook/v1.18/Upgrade/rook-upgrade/)
* [Upgrade 1.16 to 1.17](https://rook.io/docs/rook/v1.17/Upgrade/rook-upgrade/)
* [Upgrades to 1.16 or earlier](https://rook.io/docs/rook/v1.16/Upgrade/rook-upgrade/#supported-versions)

!!! important
    **Rook releases from master are expressly unsupported.** It is strongly recommended to use
    [official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
    master branch are subject to changes and incompatibilities that will not be supported in the
    official releases. Builds from the master branch can have functionality changed or removed at any
    time without compatibility support and without prior notice.

## Breaking changes in v1.21


* Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.
* Ceph msgrv2 is required by default.
    * Msgrv2 requires the 5.11 kernel. If you have an older kernel, disable the msgrv2 protocol
        with the CephCluster CR setting `network.connections.requireMsgr2: false`. If using the helm chart, this same value is applied
        under the `cephClusterSpec` of the values.
    * To preserve compatibility, existing volume mounts will continue to use the msgrv1 protocol until they are drained and remounted.
    * Ceph mons will not exclusively require msgrv2 until new mons are deployed during mon failover.
* The minimum supported Kubernetes version is v1.32.

## Considerations

With this upgrade guide, there are a few notes to consider:

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
    obstacles that damage the integrity and health of the storage cluster, including data loss.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
    both Rook operator updates and for Ceph version updates.
* Read this document in full before undertaking the cluster upgrade.

## Patch Release Upgrades

Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the common resources and the image of the Rook operator. For
example, when Rook v1.21.1 is released, the process of updating from v1.21.0 is as simple as running
the following:

```console
git clone --single-branch --depth=1 --branch v1.21.1 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If the Rook Operator or CephCluster are deployed into a different namespace than
`rook-ceph`, see the [Update common resources and CRDs](#1-update-common-resources-and-crds)
section for instructions on how to change the default namespaces in `common.yaml`.

Then, apply the latest changes from v1.21, update CSI operator resources, and update the Rook
Operator image.

```console
kubectl apply -f common.yaml -f crds.yaml -f csi-operator.yaml
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.21.1
```

A good practice is to update Rook common resources from the example
manifests before any update. The common resources and CRDs might not be updated with every
release, but Kubernetes will only apply updates to the ones that changed.

Also update optional resources like Prometheus monitoring noted more fully in the
[upgrade section below](#prometheus-updates).

## Helm

If Rook is installed via the Helm chart, Helm will handle some details of the upgrade itself.
The upgrade steps in this guide will clarify what Helm handles automatically.

!!! important
    Before upgrading to v1.20, ensure the cluster is already on at least v1.19.5.
    There was a critical update in [v1.19.5](https://github.com/rook/rook/releases/tag/v1.19.5)
    that will enable the helm upgrades to v1.20.

Upgrade charts in this order:

1. `rook-ceph`
2. `ceph-csi-drivers` (**new requirement in v1.20**)
3. `rook-ceph-cluster`

!!! note
    Be sure to update to a [supported Helm version](https://helm.sh/docs/topics/version_skew/#supported-version-skew)

### `rook-ceph` Chart

The `rook-ceph` helm chart upgrade performs the Rook operator and ceph-csi-operator subchart
upgrades.

To apply custom configuration to the ceph-csi-operator subchart, see the
[ceph-csi-operator configuration reference](https://github.com/ceph/ceph-csi-operator/blob/main/docs/helm-charts/operator-chart.md#configuration). Settings for the subchart need to be included in the
`ceph-csi-operator` section of values.yaml when creating or updating the `rook-ceph` chart.
See the default settings applied by Rook in [values.yaml](https://github.com/rook/rook/blob/release-1.20/deploy/charts/rook-ceph/values.yaml#L86).


### `ceph-csi-drivers` Chart

Install or upgrade the [`ceph-csi-drivers`](../Helm-Charts/csi-drivers-chart.md) chart,
to configure the CSI driver.

**Important points** to ensure CSI mounts continue working after upgrades:

1. This `ceph-csi-drivers` chart must be installed, otherwise the CSI driver will be in a
    failed state due to missing service accounts.
2. A recommended `values.yaml` is provided for Rook-compatible defaults in the drivers chart doc linked above.
    This is critical for setting the proper driver names with the rook operator namespace as a prefix.
3. In previous releases, CSI settings were customized in the `rook-ceph` chart.
    Now see the [CSI Drivers Chart config](https://github.com/ceph/ceph-csi-operator/blob/main/docs/helm-charts/drivers-chart.md#configuration) for customization.

### `rook-ceph-cluster` Chart

The `rook-ceph-cluster` helm chart upgrade performs a [Ceph upgrade](./ceph-upgrade.md) if the Ceph image is updated.

## Cluster Health

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster should be in a healthy state with full functionality. Review the
    [health verification guide](health-verification.md) in order to verify a CephCluster is in a good
    starting state.
* All pods consuming Rook storage should be created, running, and in a steady state.

## Rook Operator Upgrade

The examples given in this guide upgrade a live Rook cluster running `v1.19.11` to
the version `v1.21.0`. This upgrade should work from any official patch release of Rook v1.20 to any
official patch release of v1.21.

Let's get started!

### Environment

These instructions will work for as long the environment is parameterized correctly.
Set the following environment variables, which will be used throughout this document.

```console
# Parameterize the environment
export ROOK_OPERATOR_NAMESPACE=rook-ceph
export ROOK_CLUSTER_NAMESPACE=rook-ceph
```


### **1. Update common resources and CRDs**

!!! hint
    Common resources and CRDs are automatically updated when using Helm charts.

First, apply updates to Rook common resources. This includes modified privileges (RBAC) needed
by the Operator. Also update the Custom Resource Definitions (CRDs).

Get the latest common resources manifests that contain the latest changes.

```console
git clone --single-branch --depth=1 --branch master https://github.com/rook/rook.git
cd rook/deploy/examples
```

If the Rook Operator or CephCluster are deployed into a different namespace than
`rook-ceph`, update the common resource manifests to use your `ROOK_OPERATOR_NAMESPACE` and
`ROOK_CLUSTER_NAMESPACE` using `sed`.

```console
sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:operator/\1:$ROOK_OPERATOR_NAMESPACE:\2 # serviceaccount:namespace:operator/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:cluster/\1:$ROOK_CLUSTER_NAMESPACE:\2 # serviceaccount:namespace:cluster/g" \
    -e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # driver:namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE.\2 # driver:namespace:cluster/g" \
  common.yaml operator.yaml csi-operator.yaml cluster.yaml # add other files or change these as desired for your config
```

**Apply the resources.**

```console
kubectl apply -f common.yaml -f crds.yaml -f csi-operator.yaml
```

#### **Prometheus Updates**

If [Prometheus monitoring](../Storage-Configuration/Monitoring/ceph-monitoring.md) is enabled,
upgrade the Prometheus RBAC resources:

```console
kubectl apply -f deploy/examples/monitoring/rbac.yaml
```

### **2. Update the Rook Operator**

!!! hint
    The operator is automatically updated when using Helm charts.

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.21.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:master
```

### **3. Update Ceph CSI Custom Images**

!!! hint
    This is automatically updated if custom CSI image versions are not set.

Starting in v1.21, Rook no longer sets default CSI container images in the
`rook-csi-operator-image-set-configmap` configmap. All values are empty so the
ceph-csi-operator uses its built-in defaults. Users with customized CSI images
should verify overrides are still present in the configmap after upgrade.
See the [CSI Custom Images](../Storage-Configuration/Ceph-CSI/custom-images.md) documentation.

### **4. Wait for the upgrade to complete**

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, MDSes and RGWs are terminated and
replaced with updated versions in sequence. The cluster may be unresponsive very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the MDSes are upgrading. This is normal.

The versions of the components can be viewed as they are updated:

```console
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs. When all deployments report `1/1/1`
availability and `rook-version=v1.20.0`, the Ceph cluster's core components are fully updated.

```console
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.21.0
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.21.0
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.21.0
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.21.0
rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.21.0
rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.20.7
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.20.7
```

An easy check to see if the upgrade is totally finished is to check that there is only one
`rook-version` reported across the cluster.

```console
# kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.20.7
  rook-version=v1.21.0
This cluster is finished:
  rook-version=v1.21.0
```

### 5. Verify the updated cluster**

At this point, the Rook operator should be running version `rook/ceph:v1.21.0`.

Verify the CSI drivers:

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE get operatorconfig,driver
```

Verify the CephCluster health using the [health verification doc](health-verification.md).
