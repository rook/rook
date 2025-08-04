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

This guide is for upgrading from **Rook v1.17.x to Rook v1.18.x**.

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

* [Upgrade 1.16 to 1.17](https://rook.io/docs/rook/v1.17/Upgrade/rook-upgrade/)
* [Upgrade 1.15 to 1.16](https://rook.io/docs/rook/v1.16/Upgrade/rook-upgrade/)
* [Upgrade 1.14 to 1.15](https://rook.io/docs/rook/v1.15/Upgrade/rook-upgrade/)
* [Upgrade 1.13 to 1.14](https://rook.io/docs/rook/v1.14/Upgrade/rook-upgrade/)
* [Upgrade 1.12 to 1.13](https://rook.io/docs/rook/v1.13/Upgrade/rook-upgrade/)
* [Upgrade 1.11 to 1.12](https://rook.io/docs/rook/v1.12/Upgrade/rook-upgrade/)
* [Upgrade 1.10 to 1.11](https://rook.io/docs/rook/v1.11/Upgrade/rook-upgrade/)
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
    **Rook releases from master are expressly unsupported.** It is strongly recommended to use
    [official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
    master branch are subject to changes and incompatibilities that will not be supported in the
    official releases. Builds from the master branch can have functionality changed or removed at any
    time without compatibility support and without prior notice.

## Breaking changes in v1.18

* The minimum supported Kubernetes version is v1.29.

* Rook now validates node topology during CephCluster creation to prevent misconfigured CRUSH hierarchies. For example, if [topology labels](../CRDs/Cluster/ceph-cluster-crd.md#osd-topology) like `topology.rook.io/rack` are duplicated across zones, cluster creation will fail. The check applies only to new clusters. Existing clusters will only log a warning in the operator log and continue.

## Considerations

With this upgrade guide, there are a few notes to consider:

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
    obstacles that damage the integrity and health the storage cluster, including data loss.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
    both Rook operator updates and for Ceph version updates.
* Read this document in full before undertaking a Rook cluster upgrade.

## Patch Release Upgrades

Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the common resources and the image of the Rook operator. For
example, when Rook v1.18.1 is released, the process of updating from v1.18.0 is as simple as running
the following:

```console
git clone --single-branch --depth=1 --branch v1.18.1 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If the Rook Operator or CephCluster are deployed into a different namespace than
`rook-ceph`, see the [Update common resources and CRDs](#1-update-common-resources-and-crds)
section for instructions on how to change the default namespaces in `common.yaml`.

Then, apply the latest changes from v1.18, and update the Rook Operator image.

```console
kubectl apply -f common.yaml -f crds.yaml
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.18.1
```

As exemplified above, it is a good practice to update Rook common resources from the example
manifests before any update. The common resources and CRDs might not be updated with every
release, but Kubernetes will only apply updates to the ones that changed.

Also update optional resources like Prometheus monitoring noted more fully in the
[upgrade section below](#prometheus-updates).

## Helm

If Rook is installed via the Helm chart, Helm will handle some details of the upgrade itself.
The upgrade steps in this guide will clarify what Helm handles automatically.

The `rook-ceph` helm chart upgrade performs the Rook upgrade.
The `rook-ceph-cluster` helm chart upgrade performs a [Ceph upgrade](./ceph-upgrade.md) if the Ceph image is updated.
The `rook-ceph` chart should be upgraded before `rook-ceph-cluster`, so the latest operator has the opportunity to update
custom resources as necessary.

!!! note
    Be sure to update to a [supported Helm version](https://helm.sh/docs/topics/version_skew/#supported-version-skew)

## Cluster Health

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster should be in a healthy state with full functionality. Review the
    [health verification guide](health-verification.md) in order to verify a CephCluster is in a good
    starting state.
* All pods consuming Rook storage should be created, running, and in a steady state.

## Rook Operator Upgrade

The examples given in this guide upgrade a live Rook cluster running `v1.17.7` to
the version `v1.18.0`. This upgrade should work from any official patch release of Rook v1.17 to any
official patch release of v1.18.

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
  common.yaml
```

**Apply the resources.**

```console
kubectl apply -f common.yaml -f crds.yaml
```

#### **Prometheus Updates**

If [Prometheus monitoring](../Storage-Configuration/Monitoring/ceph-monitoring.md) is enabled,
follow this step to upgrade the Prometheus RBAC resources as well.

```console
kubectl apply -f deploy/examples/monitoring/rbac.yaml
```

### **2. Update the Rook Operator**

!!! hint
    The operator is automatically updated when using Helm charts.

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.18.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:master
```

### **3. Update Ceph CSI**

!!! hint
    This is automatically updated if custom CSI image versions are not set.

Update to the latest Ceph-CSI drivers if custom CSI images are specified.
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
availability and `rook-version=v1.17.0`, the Ceph cluster's core components are fully updated.

```console
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.18.0
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.18.0
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.18.0
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.18.0
rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.18.0
rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.17.7
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.17.7
```

An easy check to see if the upgrade is totally finished is to check that there is only one
`rook-version` reported across the cluster.

```console
# kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.17.7
  rook-version=v1.18.0
This cluster is finished:
  rook-version=v1.18.0
```

### **5. Verify the updated cluster**

At this point, the Rook operator should be running version `rook/ceph:v1.18.0`.

Verify the CephCluster health using the [health verification doc](health-verification.md).
