---
title: Upgrades
weight: 3800
indent: true
---

{% include_relative branch.liquid %}

# Rook-Ceph Upgrades

This guide will walk you through the steps to upgrade the software in a Rook-Ceph cluster from one
version to the next. This includes both the Rook-Ceph operator software itself as well as the Ceph
cluster software.

Upgrades for both the operator and for Ceph are nearly entirely automated save for where Rook's
permissions need to be explicitly updated by an admin or when incompatibilities need to be addressed
manually due to customizations.

We welcome feedback and opening issues!

## Supported Versions

This guide is for upgrading from **Rook v1.8.x to Rook v1.9.x**.

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases. Upgrades to and from `master` are not
supported.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

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

## Breaking changes in this release

* Helm charts now define default resource requests and limits for Rook-Ceph Pods. If you use Helm,
  ensure you have defined an override for these in your `values.yaml` if you don't wish to use the
  recommended defaults. Setting resource requests and limits could mean that Kubernetes will not
  allow Pods to be scheduled in some cases. If sufficient resources are not available, you can
  reduce or remove the requests and limits.

* MDS liveness and startup probes are now configured by the CephFilesystem resource instead of
  CephCluster. Upgrade instructions are [below](#mds-liveness-and-startup-probes).

* Rook no longer deploys Prometheus rules from the operator. If you have been relying on Rook to
  deploy prometheus rules in the past, please follow the upgrade instructions [below](#prometheus).

* Due to a number of Ceph issues and changes, Rook officially only supports Ceph
  v16.2.7 or higher for CephNFS. If you are using an earlier version, upgrade your Ceph version
  following the advice given in Rook's
  [v1.8 NFS docs](https://rook.github.io/docs/rook/v1.8/ceph-nfs-crd.html).

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
example, when Rook v1.9.1 is released, the process of updating from v1.9.0 is as simple as running
the following:

First get the latest common resources manifests that contain the latest changes for Rook v1.9.
```sh
git clone --single-branch --depth=1 --branch v1.9.1 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If you have deployed the Rook Operator or the Ceph cluster into a different namespace than
`rook-ceph`, see the [Update common resources and CRDs](#1-update-common-resources-and-crds)
section for instructions on how to change the default namespaces in `common.yaml`.

Then apply the latest changes from v1.9 and update the Rook Operator image.
```console
kubectl apply -f common.yaml -f crds.yaml
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.9.1
```

As exemplified above, it is a good practice to update Rook-Ceph common resources from the example
manifests before any update. The common resources and CRDs might not be updated with every
release, but K8s will only apply updates to the ones that changed.

Also update optional resources like Prometheus monitoring noted more fully in the
[upgrade section below](#updates-for-optional-resources).

## Helm

* The minimum supported Helm version is **v3.2.0**

If you have installed Rook via the Helm chart, Helm will handle some details of the upgrade for you.
The upgrade steps in this guide will clarify if Helm manages the step for you.

The `rook-ceph` helm chart upgrade performs the Rook upgrade.
The `rook-ceph-cluster` helm chart upgrade performs a [Ceph upgrade](#ceph-version-upgrades) if the Ceph image is updated.

## Upgrading from v1.8 to v1.9

**Rook releases from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed or removed at any
time without compatibility support and without prior notice.

### **Prerequisites**

We will do all our work in the Ceph example manifests directory.

```sh
$ cd $YOUR_ROOK_REPO/deploy/examples/
```

Unless your Rook cluster was created with customized namespaces, namespaces for Rook clusters are
likely to be:

* Clusters created by v0.7 or earlier: `rook-system` and `rook`
* Clusters created in v0.8 or v0.9: `rook-ceph-system` and `rook-ceph`
* Clusters created in v1.0 or newer: only `rook-ceph`

With this guide, we do our best not to assume the namespaces in your cluster. To make things as easy
as possible, modify and use the below snippet to configure your environment. We will use these
environment variables throughout this document.

```sh
# Parameterize the environment
export ROOK_OPERATOR_NAMESPACE="rook-ceph"
export ROOK_CLUSTER_NAMESPACE="rook-ceph"
```

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster should be in a healthy state with full functionality. Review the
  [health verification section](#health-verification) in order to verify your cluster is in a good
  starting state.
* All pods consuming Rook storage should be created, running, and in a steady state. No Rook
  persistent volumes should be in the act of being created or deleted.

## Health Verification

Before we begin the upgrade process, let's first review some ways that you can verify the health of
your cluster, ensuring that the upgrade is going smoothly after each step. Most of the health
verification checks for your cluster during the upgrade process can be performed with the Rook
toolbox. For more information about how to run the toolbox, please visit the
[Rook toolbox readme](./ceph-toolbox.md).

See the common issues pages for troubleshooting and correcting health issues:

* [General troubleshooting](./common-issues.md)
* [Ceph troubleshooting](./ceph-common-issues.md)

### **Pods all Running**

In a healthy Rook cluster, the operator, the agents and all Rook namespace pods should be in the
`Running` state and have few, if any, pod restarts. To verify this, run the following commands:

```sh
kubectl -n $ROOK_CLUSTER_NAMESPACE get pods
```

### **Status Output**

The Rook toolbox contains the Ceph tools that can give you status details of the cluster with the
`ceph status` command. Let's look at an output sample and review some of the details:

```sh
TOOLS_POD=$(kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[*].metadata.name}')
kubectl -n $ROOK_CLUSTER_NAMESPACE exec -it $TOOLS_POD -- ceph status
```

>```
>  cluster:
>    id:     a3f4d647-9538-4aff-9fd1-b845873c3fe9
>    health: HEALTH_OK
>
>  services:
>    mon: 3 daemons, quorum b,c,a
>    mgr: a(active)
>    mds: myfs-1/1/1 up  {0=myfs-a=up:active}, 1 up:standby-replay
>    osd: 6 osds: 6 up, 6 in
>    rgw: 1 daemon active
>
>  data:
>    pools:   9 pools, 900 pgs
>    objects: 67  objects, 11 KiB
>    usage:   6.1 GiB used, 54 GiB / 60 GiB avail
>    pgs:     900 active+clean
>
>  io:
>    client:   7.4 KiB/s rd, 681 B/s wr, 11 op/s rd, 4 op/s wr
>    recovery: 164 B/s, 1 objects/s
>```

In the output above, note the following indications that the cluster is in a healthy state:

* Cluster health: The overall cluster status is `HEALTH_OK` and there are no warning or error status
  messages displayed.
* Monitors (mon):  All of the monitors are included in the `quorum` list.
* Manager (mgr): The Ceph manager is in the `active` state.
* OSDs (osd): All OSDs are `up` and `in`.
* Placement groups (pgs): All PGs are in the `active+clean` state.
* (If applicable) Ceph filesystem metadata server (mds): all MDSes are `active` for all filesystems
* (If applicable) Ceph object store RADOS gateways (rgw): all daemons are `active`

If your `ceph status` output has deviations from the general good health described above, there may
be an issue that needs to be investigated further. There are other commands you may run for more
details on the health of the system, such as `ceph osd status`. See the
[Ceph troubleshooting docs](https://docs.ceph.com/docs/master/rados/troubleshooting/) for help.

Rook will prevent the upgrade of the Ceph daemons if the health is in a `HEALTH_ERR` state.
If you desired to proceed with the upgrade anyway, you will need to set either
`skipUpgradeChecks: true` or `continueUpgradeAfterChecksEvenIfNotHealthy: true` as described in the
[cluster CR settings](ceph-cluster-crd.md#cluster-settings).

### **Container Versions**

The container version running in a specific pod in the Rook cluster can be verified in its pod spec
output. For example for the monitor pod `mon-b`, we can verify the container version it is running
with the below commands:

```sh
POD_NAME=$(kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon-b)
kubectl -n $ROOK_CLUSTER_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

The status and container versions for all Rook pods can be collected all at once with the following
commands:

```sh
kubectl -n $ROOK_OPERATOR_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}' && \
kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0].image}{"\n"}{end}'
```

The `rook-version` label exists on Ceph controller resources. For various resource controllers, a
summary of the resource controllers can be gained with the commands below. These will report the
requested, updated, and currently available replicas for various Rook-Ceph resources in addition to
the version of Rook for resources managed by the updated Rook-Ceph operator. Note that the operator
and toolbox deployments do not have a `rook-version` label set.

```sh
kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_CLUSTER_NAMESPACE get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"  \tsucceeded: "}{.status.succeeded}{"      \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

### **Rook Volume Health**

Any pod that is using a Rook volume should also remain healthy:

* The pod should be in the `Running` state with few, if any, restarts
* There should be no errors in its logs
* The pod should still be able to read and write to the attached Rook volume.

## Rook Operator Upgrade Process

In the examples given in this guide, we will be upgrading a live Rook cluster running `v1.8.8` to
the version `v1.9.0`. This upgrade should work from any official patch release of Rook v1.8 to any
official patch release of v1.9.

**Rook releases from `master` are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed or removed at any
time without compatibility support and without prior notice.

These methods should work for any number of Rook-Ceph clusters and Rook Operators as long as you
parameterize the environment correctly. Merely repeat these steps for each Rook-Ceph cluster
(`ROOK_CLUSTER_NAMESPACE`), and be sure to update the `ROOK_OPERATOR_NAMESPACE` parameter each time
if applicable.

Let's get started!

### **1. Update common resources and CRDs**

> Automatically updated if you are upgrading via the helm chart

First apply updates to Rook-Ceph common resources. This includes modified privileges (RBAC) needed
by the Operator. Also update the Custom Resource Definitions (CRDs).

Get the latest common resources manifests that contain the latest changes.
```sh
git clone --single-branch --depth=1 --branch v1.9.0 https://github.com/rook/rook.git
cd rook/deploy/examples
```

If you have deployed the Rook Operator or the Ceph cluster into a different namespace than
`rook-ceph`, update the common resource manifests to use your `ROOK_OPERATOR_NAMESPACE` and
`ROOK_CLUSTER_NAMESPACE` using `sed`.
```sh
sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
  common.yaml
```

Then apply the latest changes.
```sh
kubectl apply -f common.yaml -f crds.yaml
```

#### **Updates for optional resources**

#### **Prometheus**

If you have [Prometheus monitoring](ceph-monitoring.md) enabled, follow the
step to upgrade the Prometheus RBAC resources as well.

```sh
kubectl apply -f deploy/examples/monitoring/rbac.yaml
```

Rook no longer deploys Prometheus rules from the operator.

If you use the Helm chart `monitoring.enabled` value to deploy Prometheus rules, you may now
additionally use `monitoring.createPrometheusRules` to instruct Helm to deploy the rules. You may
alternately deploy the rules manually if you wish.

To see the latest information about manually deploying rules, see the
[Prometheus monitoring docs](ceph-monitoring.md#prometheus-alets).

#### **MDS liveness and startup probes**

If you configure MDS probes in the CephCluster resource, copy them to the
[CephFilesystem `metadataServer` settings](ceph-filesystem-crd.md#metadata-server-settings) at this
point. Do not remove them from the CephCluster until after the Rook upgrade is fully complete.

### **2. Update Ceph CSI versions**

> Automatically updated if you are upgrading via the helm chart

If you have specified custom CSI images in the Rook-Ceph Operator deployment, we recommended you
update to use the latest Ceph-CSI drivers. See the [CSI Version](#csi-version) section for more
details.

> Note: If using snapshots, refer to the [Upgrade Snapshot API guide](ceph-csi-snapshot.md#upgrade-snapshot-api).

### **3. Update the Rook Operator**

> Automatically updated if you are upgrading via the helm chart

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.9.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.

```sh
kubectl -n $ROOK_OPERATOR_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.9.0
```

### **4. Wait for the upgrade to complete**

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, MDSes and RGWs are terminated and
replaced with updated versions in sequence. The cluster may be offline very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the MDSes are upgrading. This is normal.

The versions of the components can be viewed as they are updated:

```sh
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs. When all deployments report `1/1/1`
availability and `rook-version=v1.9.0`, the Ceph cluster's core components are fully updated.

>```
>Every 2.0s: kubectl -n rook-ceph get deployment -o j...
>
>rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.9.0
>rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.9.0
>rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.9.0
>rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.9.0
>rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.9.0
>rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.8.8
>rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.8.8
>```

An easy check to see if the upgrade is totally finished is to check that there is only one
`rook-version` reported across the cluster.

```console
# kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.8.8
  rook-version=v1.9.0
This cluster is finished:
  rook-version=v1.9.0
```

### **5. Verify the updated cluster**

At this point, your Rook operator should be running version `rook/ceph:v1.9.0`.

Verify the Ceph cluster's health using the [health verification section](#health-verification).


## Ceph Version Upgrades

Rook v1.9 supports the following Ceph versions:
- Ceph Quincy v17.2.0 or newer
- Ceph Pacific v16.2.0 or newer
- Ceph Octopus v15.2.0 or newer

These are the only supported versions of Ceph. Rook v1.10 is planning to drop support for Ceph
Octopus (15.2.x), so please consider upgrading your Ceph cluster.

> **IMPORTANT: When an update is requested, the operator will check Ceph's status, if it is in `HEALTH_ERR` it will refuse to do the upgrade.**

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been
updated in the CR), Rook will go through all the daemons one by one and will individually perform
checks on them. It will make sure a particular daemon can be stopped before performing the upgrade.
Once the deployment has been updated, it checks if this is ok to continue. After each daemon is
updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for
MDSes, etc.), then only when the condition is met we move to the next daemon. We repeat this process
until all the daemons have been updated.

We recommend updating to v16.2.7 or newer. If you require updating **to v16.2.0-v16.2.6**,
please see the [v1.8 upgrade guide for a special upgrade consideration](https://rook.github.io/docs/rook/v1.8/ceph-upgrade.html#disable-bluestore_fsck_quick_fix_on_mount).

### **Rename CephBlockPool device_health_metrics pool when upgrading to Quincy v17**
In Ceph Quincy (v17), the `device_health_metrics` pool was renamed to `.mgr`. Ceph will perform this
migration automatically. If you do not use CephBlockPool to customize the configuration of the
`device_health_metrics` pool, you don't need to do anything further here.

If you do use CephBlockPool to customize the configuration of the `device_health_metrics` pool, you
will need two extra steps after the Ceph upgrade is complete. Once upgrade is complete:
1. Create a new CephBlockPool to configure the `.mgr` built-in pool. You can reference the example
[builtin mgr pool](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/pool-builtin-mgr.yaml).
2. Delete the old CephBlockPool that represents the `device_health_metrics` pool.

### **Important consideration for CephNFS users**
Ceph Quincy v17.2.0 has a potentially breaking regression with CephNFS. See the NFS documentation's
[known issue](ceph-nfs-crd.md#ceph-v1720) for more detail.

### **Ceph images**

Official Ceph container images can be found on [Quay](https://quay.io/repository/ceph/ceph?tab=tags).
Prior to August 2021, official images were on docker.io. While those images will remain on Docker Hub, all new images are being pushed to Quay.

These images are tagged in a few ways:

* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v16.2.7-20220216`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v16`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

### **Example upgrade to Ceph Pacific**

#### **1. Update the main Ceph daemons**

The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
Ceph image field in the cluster CRD (`spec.cephVersion.image`).

```sh
NEW_CEPH_IMAGE='quay.io/ceph/ceph:v16.2.7-20220216'
CLUSTER_NAME="$ROOK_CLUSTER_NAMESPACE"  # change if your cluster name is not the Rook namespace
kubectl -n $ROOK_CLUSTER_NAMESPACE patch CephCluster $CLUSTER_NAME --type=merge -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

#### **2. Wait for the daemon pod updates to complete**

As with upgrading Rook, you must now wait for the upgrade to complete. Status can be determined in a
similar way to the Rook upgrade as well.

```sh
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \tceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}'
```

Determining when the Ceph has fully updated is rather simple.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"ceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
    ceph-version=15.2.13-0
    ceph-version=16.2.6-0
This cluster is finished:
    ceph-version=16.2.6-0
```

#### **3. Verify the updated cluster**

Verify the Ceph cluster's health using the [health verification section](#health-verification).


## CSI Version

If you have a cluster running with CSI drivers enabled and you want to configure Rook
to use non-default CSI images, the following settings will need to be applied for the desired
version of CSI.

The operator configuration variables are found in the `rook-ceph-operator-config` ConfigMap.
These settings can also be specified as environment variables on the operator deployment, but
the env vars will be overridden if configmap values are specified.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE edit configmap rook-ceph-operator-config
```

The default upstream images are included below, which you can change to your desired images.

```yaml
ROOK_CSI_CEPH_IMAGE: "quay.io/cephcsi/cephcsi:v3.6.1"
ROOK_CSI_REGISTRAR_IMAGE: "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.5.0"
ROOK_CSI_PROVISIONER_IMAGE: "k8s.gcr.io/sig-storage/csi-provisioner:v3.1.0"
ROOK_CSI_ATTACHER_IMAGE: "k8s.gcr.io/sig-storage/csi-attacher:v3.4.0"
ROOK_CSI_RESIZER_IMAGE: "k8s.gcr.io/sig-storage/csi-resizer:v1.4.0"
ROOK_CSI_SNAPSHOTTER_IMAGE: "k8s.gcr.io/sig-storage/csi-snapshotter:v5.0.1"
CSI_VOLUME_REPLICATION_IMAGE: "quay.io/csiaddons/volumereplication-operator:v0.3.0"
ROOK_CSIADDONS_IMAGE: "quay.io/csiaddons/k8s-sidecar:v0.2.1"
```

### **Use default images**

If you would like Rook to use the inbuilt default upstream images, then you may simply remove all
variables matching `ROOK_CSI_*_IMAGE` from the above ConfigMap and/or the operator deployment.

### **Verifying updates**

You can use the below command to see the CSI images currently being used in the cluster. Note that
not all images (like `volumereplication-operator`) may be present in every cluster depending on
which CSI features are enabled.

```console
kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app in (csi-rbdplugin,csi-rbdplugin-provisioner,csi-cephfsplugin,csi-cephfsplugin-provisioner)' | sort | uniq
```

The default images can also be found with each release in the [images list](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/images.txt)
