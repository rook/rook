---
title: Rook Upgrades
---

This guide will walk you through the steps to upgrade the software in a Rook cluster from one
version to the next. This guide focuses on updating the Rook version for the management layer,
while the [Ceph upgrade](ceph-upgrade.md) guide focuses on updating the data layer.

Upgrades for both the operator and for Ceph are entirely automated except where Rook's
permissions need to be explicitly updated by an admin or when incompatibilities need to be addressed
manually due to customizations.

We welcome feedback and opening issues!

## Supported Versions

This guide is for upgrading from **Rook v1.10.x to Rook v1.11.x**.

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

* [Upgrade 1.9 to 1.10](https://rook.io/docs/rook/v1.10/Upgrade/rook-upgrade/)
* [Upgrade 1.8 to 1.9](https://rook.io/docs/rook/v1.9/Upgrade/rook-upgrade/)
* [Upgrade 1.7 to 1.8](https://rook.io/docs/rook/v1.8/ceph-upgrade.html)
* [Upgrade 1.6 to 1.7](https://rook.io/docs/rook/v1.7/ceph-upgrade.html)
* [Upgrade 1.5 to 1.6](https://rook.io/docs/rook/v1.6/ceph-upgrade.html)
* [Upgrade 1.4 to 1.5](https://rook.io/docs/rook/v1.5/ceph-upgrade.html)
* [Upgrade 1.3 to 1.4](https://rook.io/docs/rook/v1.4/ceph-upgrade.html)
* [Upgrade 1.2 to 1.3](https://rook.io/docs/rook/v1.3/ceph-upgrade.html)
* [Upgrade 1.1 to 1.2](https://rook.io/docs/rook/v1.2/ceph-upgrade.html)
* [Upgrade 1.0 to 1.1](https://rook.io/docs/rook/v1.1/ceph-upgrade.html)
* [Upgrade 0.9 to 1.0](https://rook.io/docs/rook/v1.0/ceph-upgrade.html)
* [Upgrade 0.8 to 0.9](https://rook.io/docs/rook/v0.9/ceph-upgrade.html)
* [Upgrade 0.7 to 0.8](https://rook.io/docs/rook/v0.8/upgrade.html)
* [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
* [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)

!!! important
    **Rook releases from master are expressly unsupported.** It is strongly recommended that you use
    [official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
    master branch are subject to changes and incompatibilities that will not be supported in the
    official releases. Builds from the master branch can have functionality changed or removed at any
    time without compatibility support and without prior notice.

## Breaking changes in v1.11

* The minimum supported version of Kubernetes is now v1.21.0

## Considerations

With this upgrade guide, there are a few notes to consider:

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
  both Rook operator updates and for Ceph version updates.
* We recommend that you read this document in full before you undertake a Rook cluster upgrade.

## Patch Release Upgrades

Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the common resources and the image of the Rook operator. For
example, when Rook v1.11.1 is released, the process of updating from v1.11.0 is as simple as running
the following:

```console
git clone --single-branch --depth=1 --branch v1.11.1 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If you have deployed the Rook Operator or the Ceph cluster into a different namespace than
`rook-ceph`, see the [Update common resources and CRDs](#1-update-common-resources-and-crds)
section for instructions on how to change the default namespaces in `common.yaml`.

Then apply the latest changes from v1.11 and update the Rook Operator image.

```console
kubectl apply --server-side -f common.yaml -f crds.yaml
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.11.1
```

As exemplified above, it is a good practice to update Rook common resources from the example
manifests before any update. The common resources and CRDs might not be updated with every
release, but K8s will only apply updates to the ones that changed.

Also update optional resources like Prometheus monitoring noted more fully in the
[upgrade section below](#updates-for-optional-resources).

## Helm

If you have installed Rook via the Helm chart, Helm will handle some details of the upgrade for you.
The upgrade steps in this guide will clarify if Helm manages the step for you.

The `rook-ceph` helm chart upgrade performs the Rook upgrade.
The `rook-ceph-cluster` helm chart upgrade performs a [Ceph upgrade](#ceph-version-upgrades) if the Ceph image is updated.

!!! note
    Be sure to update to a [supported Helm version](https://helm.sh/docs/topics/version_skew/#supported-version-skew)

## Cluster Health

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster should be in a healthy state with full functionality. Review the
  [health verification guide](health-verification.md) in order to verify your cluster is in a good
  starting state.
* All pods consuming Rook storage should be created, running, and in a steady state.

## Rook Operator Upgrade

In the examples given in this guide, we will be upgrading a live Rook cluster running `v1.10.11` to
the version `v1.11.0`. This upgrade should work from any official patch release of Rook v1.10 to any
official patch release of v1.11.

Let's get started!

### Environment

These instructions will work for as long as you parameterize the environment correctly.
With this guide, we do our best not to assume the namespaces in your cluster.
Set the following environment variables, which will be used throughout this document.

```console
# Parameterize the environment
export ROOK_OPERATOR_NAMESPACE=rook-ceph
export ROOK_CLUSTER_NAMESPACE=rook-ceph
```

### **1. Update common resources and CRDs**

!!! hint
    If you are upgrading via the Helm chart, the common resources and CRDs are automatically updated.

First apply updates to Rook common resources. This includes modified privileges (RBAC) needed
by the Operator. Also update the Custom Resource Definitions (CRDs).

Get the latest common resources manifests that contain the latest changes.

```console
git clone --single-branch --depth=1 --branch v1.11.0-beta.0 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If you have deployed the Rook Operator or the Ceph cluster into a different namespace than
`rook-ceph`, update the common resource manifests to use your `ROOK_OPERATOR_NAMESPACE` and
`ROOK_CLUSTER_NAMESPACE` using `sed`.

```console
sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
  common.yaml
```

**Apply the resources.**

```console
kubectl apply --server-side -f common.yaml -f crds.yaml
```

#### **Prometheus Updates**

If you have [Prometheus monitoring](../Storage-Configuration/Monitoring/ceph-monitoring.md) enabled, follow the
step to upgrade the Prometheus RBAC resources as well.

```console
kubectl apply -f deploy/examples/monitoring/rbac.yaml
```

### **2. Update the Rook Operator**

!!! hint
    If you are upgrading via the Helm chart, the operator is automatically updated.

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.11.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.11.0-beta.0
```

### **3. Update Ceph CSI**

!!! hint
    If have not customized the CSI image versions, this is automatically updated.

!!! important
    The minimum supported version of Ceph-CSI is v3.7.0.

If you have specified custom CSI images, we recommended you update to the latest Ceph-CSI drivers.
See the [CSI Custom Images](../Storage-Configuration/Ceph-CSI/custom-images.md) documentation.

!!! note
    If using snapshots, refer to the [Upgrade Snapshot API guide](../Storage-Configuration/Ceph-CSI/ceph-csi-snapshot.md#upgrade-snapshot-api).

### **4. Wait for the upgrade to complete**

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, MDSes and RGWs are terminated and
replaced with updated versions in sequence. The cluster may be unresponsive very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the MDSes are upgrading. This is normal.

The versions of the components can be viewed as they are updated:

```console
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs. When all deployments report `1/1/1`
availability and `rook-version=v1.11.0`, the Ceph cluster's core components are fully updated.

```console
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.11.0
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.11.0
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.11.0
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.11.0
rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.11.0
rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.10.11
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.10.11
```

An easy check to see if the upgrade is totally finished is to check that there is only one
`rook-version` reported across the cluster.

```console
# kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.10.11
  rook-version=v1.11.0
This cluster is finished:
  rook-version=v1.11.0
```

### **5. Verify the updated cluster**

At this point, your Rook operator should be running version `rook/ceph:v1.11.0`.

Verify the Ceph cluster's health using the [health verification doc](health-verification.md).
