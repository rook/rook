---
title: Upgrades
weight: 3800
indent: true
---

# Rook-Ceph Upgrades

This guide will walk you through the steps to upgrade the software in a Rook-Ceph cluster from one
version to the next. This includes both the Rook-Ceph operator software itself as well as the Ceph
cluster software.

Since the release of Rook 1.0, upgrades for both the operator and for Ceph are nearly entirely
automated save for where Rook's permissions need to be explicitly updated by an admin. Achieving the
level of upgrade automation has been refined by community feedback, and we will always be open to
further feedback for improving automation and improving Rook.

We welcome feedback and opening issues!

## Supported Versions

This guide is for upgrading from **Rook v1.2.x to Rook v1.3.x**.

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases. Upgrades to and from `master` are not
supported.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

* [Upgrade 1.1 to 1.2](https://rook.io/docs/rook/v1.2/ceph-upgrade.html)
* [Upgrade 1.0 to 1.1](https://rook.io/docs/rook/v1.1/ceph-upgrade.html)
* [Upgrade 0.9 to 1.0](https://rook.io/docs/rook/v1.0/ceph-upgrade.html)
* [Upgrade 0.8 to 0.9](https://rook.io/docs/rook/v0.9/ceph-upgrade.html)
* [Upgrade 0.7 to 0.8](https://rook.io/docs/rook/v0.8/upgrade.html)
* [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
* [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)

## Considerations

With this upgrade guide, there are a few notes to consider:

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss. Only
  proceed with this guide if you are comfortable with that.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
  both Rook operator updates and for Ceph version updates.
* We recommend that you read this document in full before you undertake a Rook cluster upgrade.

## Before you Upgrade

Rook v1.3 has several breaking changes that must be considered **before** upgrading to Rook v1.3.
1. Ceph Mimic is no longer supported. You must update to Ceph Nautilus v14.2.5 or newer **before** updating to Rook v1.3. See the
   Rook v1.2 [Upgrade Guide for Ceph](https://rook.github.io/docs/rook/v1.2/ceph-upgrade.html#ceph-version-upgrades).
2. Directory-based OSDs are no longer supported. If you are specifying a
   [`directory`](https://github.com/rook/rook/blob/release-1.2/cluster/examples/kubernetes/ceph/cluster-test.yaml#L49-L50)
   in your CephCluster CR you will need to convert these OSDs to device-based OSDs. See below for [converting legacy OSDs](#converting-legacy-osds).
3. OSDs created **before Rook v0.9** are no longer supported during upgrade. To detect if you have these legacy
   OSDs in your cluster, run `lsblk` on each host to see what partitions exist.
   - If you see **three partitions** on a device running an OSD, this is a legacy OSD. See below for [converting legacy OSDs](#converting-legacy-osds).
   - If you see a single partition or LV with the `ceph` prefix, the OSD is **not** a legacy OSD and you will not need to convert the OSDs.

### Converting Legacy OSDs

If you have legacy OSDs as described in the previous section, it is recommended to migrate the OSDs before the upgrade,
although the migration is not strictly required before upgrade. If you proceed with the upgrade to v1.3 the OSDs will
continue running. However, they will no longer be managed by Rook. The OSDs will never again be updated when you update
Ceph or any other settings in your CephCluster CR.

The procedure to migrate the OSDs requires raw devices or partitions be available as mentioned in the [Ceph Prerequisites](ceph-prerequisites.md).
After you have available storage devices available in your cluster, do the following:
1. Configure OSDs on the new devices using the [Storage selection settings](https://rook.github.io/docs/rook/v1.3/ceph-cluster-crd.html#storage-selection-settings).
   If you have specified `useAllDevices: true`, you may need to restart the operator in order to trigger creation of new OSDs on new devices.
2. Follow the [OSD Removal Guide](https://rook.github.io/docs/rook/v1.3/ceph-osd-mgmt.html#remove-an-osd) to remove the legacy OSDs.
   - Before removing each OSD, ensure the PGs are all `active+clean` before continuing with the next OSD to ensure your data is safe.

If you need to re-use your devices and add OSDs to the same devices, you'll need to zap a device before creating a new
OSD on the same device. For steps on zapping the device see the [Rook cleanup instructions](ceph-teardown.md#zapping-devices).

> **Removing OSDs and zapping devices is destructive so proceed with extreme caution**

## Upgrading the Rook-Ceph Operator

## Patch Release Upgrades

Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the image of the Rook operator. For example, when Rook v1.3.11 is
released, the process of updating from v1.3.0 is as simple as running the following:

```console
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.3.11
```

## Helm Upgrades

If you have installed Rook via the Helm chart, Helm will handle some details of the upgrade for you.
In particular, Helm will handle updating the RBAC and trigger the operator update.
The details expected to be handled by Helm will be noted in each section as follows:

> Automatically updated if you are upgrading via the helm chart

## Upgrading from v1.2 to v1.3

**Rook releases from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed and even removed at
any time without compatibility support and without prior notice.

## Prerequisites

We will do all our work in the Ceph example manifests directory.

```sh
cd $YOUR_ROOK_REPO/cluster/examples/kubernetes/ceph/
```

Unless your Rook cluster was created with customized namespaces, namespaces for Rook clusters
created before v0.8 are likely to be:

* Clusters created by v0.7 or earlier: `rook-system` and `rook`
* Clusters created in v0.8 or v0.9: `rook-ceph-system` and `rook-ceph`
* Clusters created in v1.0 or newer: only `rook-ceph`

With this guide, we do our best not to assume the namespaces in your cluster. To make things as easy
as possible, modify and use the below snippet to configure your environment. We will use these
environment variables throughout this document.

```sh
# Parameterize the environment
export ROOK_SYSTEM_NAMESPACE="rook-ceph"
export ROOK_NAMESPACE="rook-ceph"
```

You must also update one of the upgrade manifests to set the namespace for your
unique cluster if you aren't using the default `rook-ceph` namespace above.
```sh
sed -i.bak "s/namespace: rook-ceph/namespace: $ROOK_NAMESPACE/g" upgrade-from-v1.2-apply.yaml
```

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster should be in a healthy state with full functionality. Review the
  [health verification section](#health-verification) in order to verify your cluster is in a good
  starting state.
* All pods consuming Rook storage should be created, running, and in a steady state. No Rook
  persistent volumes should be in the act of being created or deleted.
* Your Helm version should be newer than v3.2.0 for avoiding [this issue](https://github.com/helm/helm/issues/7697).
* For the already deployed Rook cluster with the Helm older than v3.2.0, you also need to execute the following commands.

```sh
KIND=ClusterRole
NAME=psp:rook
RELEASE=your-apps-release-name
NAMESPACE=your-apps-namespace
kubectl annotate $KIND $NAME meta.helm.sh/release-name=$RELEASE
kubectl annotate $KIND $NAME meta.helm.sh/release-namespace=$NAMESPACE
kubectl label $KIND $NAME app.kubernetes.io/managed-by=Helm
```

## Health Verification

Before we begin the upgrade process, let's first review some ways that you can verify the health of
your cluster, ensuring that the upgrade is going smoothly after each step. Most of the health
verification checks for your cluster during the upgrade process can be performed with the Rook
toolbox. For more information about how to run the toolbox, please visit the
[Rook toolbox readme](./ceph-toolbox.md#running-the-toolbox-in-kubernetes).

See the common issues pages for troubleshooting and correcting health issues:

* [General troubleshooting](./common-issues.md)
* [Ceph troubleshooting](./ceph-common-issues.md)

### Pods all Running

In a healthy Rook cluster, the operator, the agents and all Rook namespace pods should be in the
`Running` state and have few, if any, pod restarts. To verify this, run the following commands:

```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE get pods
kubectl -n $ROOK_NAMESPACE get pods
```

### Status Output

The Rook toolbox contains the Ceph tools that can give you status details of the cluster with the
`ceph status` command. Let's look at an output sample and review some of the details:

```sh
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph status
```
```console
  cluster:
    id:     a3f4d647-9538-4aff-9fd1-b845873c3fe9
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum b,c,a
    mgr: a(active)
    mds: myfs-1/1/1 up  {0=myfs-a=up:active}, 1 up:standby-replay
    osd: 6 osds: 6 up, 6 in
    rgw: 1 daemon active

  data:
    pools:   9 pools, 900 pgs
    objects: 67  objects, 11 KiB
    usage:   6.1 GiB used, 54 GiB / 60 GiB avail
    pgs:     900 active+clean

  io:
    client:   7.4 KiB/s rd, 681 B/s wr, 11 op/s rd, 4 op/s wr
    recovery: 164 B/s, 1 objects/s
```

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

### Container Versions

The container version running in a specific pod in the Rook cluster can be verified in its pod spec
output. For example for the monitor pod `mon-b`, we can verify the container version it is running
with the below commands:

```sh
POD_NAME=$(kubectl -n $ROOK_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon-b)
kubectl -n $ROOK_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

### All Pods Status and Version

The status and container versions for all Rook pods can be collected all at once with the following
commands:

```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}' && \
kubectl -n $ROOK_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0].image}{"\n"}{end}'
```

The `rook-version` label exists on Ceph controller resources. For various resource controllers, a
summary of the resource controllers can be gained with the commands below. These will report the
requested, updated, and currently available replicas for various Rook-Ceph resources in addition to
the version of Rook for resources managed by the updated Rook-Ceph operator. Note that the operator
and toolbox deployments do not have a `rook-version` label set.

```sh
kubectl -n $ROOK_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_NAMESPACE get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"  \tsucceeded: "}{.status.succeeded}{"      \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

### Rook Volume Health

Any pod that is using a Rook volume should also remain healthy:

* The pod should be in the `Running` state with few, if any, restarts
* There should be no errors in its logs
* The pod should still be able to read and write to the attached Rook volume.

## Rook Operator Upgrade Process

In the examples given in this guide, we will be upgrading a live Rook cluster running `v1.2.7` to
the version `v1.3.11`. This upgrade should work from any official patch release of Rook v1.2 to any
official patch release of v1.3. We will further assume that your previous cluster was created using
an earlier version of this guide and manifests. If you have created custom manifests, these steps
may not work as written.

**Rook release from `master` are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed or removed at any
time without compatibility support and without prior notice.

Let's get started!

## 0. Update Ceph to Nautilus version 14.2.5 or higher

As described above in the [Before You Upgrade](#before-you-upgrade) section, you must upgrade to
Nautilus version 14.2.5 or higher before upgrading to Rook v1.3.

## 1. Migrate Legacy OSDs

As described above in the [Before You Upgrade](#before-you-upgrade) section, it is recommended to migrate legacy OSDs
before upgrading to Rook v1.3.

## 2. Update the RBAC and CRDs

> Automatically updated if you are upgrading via the helm chart

First apply new resources. This includes slightly modified privileges (RBAC) needed by the Operator.
Also update Ceph Custom Resource Definitions (CRDs) at this time. Many CRDs have had a `status` item
added to them which must be present for Rook v1.3 to update CRD statuses.

```sh
kubectl apply -f upgrade-from-v1.2-apply.yaml -f upgrade-from-v1.2-crds.yaml
```

## 3. Update Ceph CSI version to v2.0

If you have specified custom CSI images in the Rook-Ceph Operator deployment, you should update your
the deployment to use the latest Ceph-CSI v2.0 and related drivers. If you have not specified custom
CSI images in the Operator deployment this step is unnecessary since Rook v1.3 will use the latest
drivers by default.

See the section [CSI Updates](#csi-updates) for details about how to do this.

## 4. Update the Rook Operator

> Automatically updated if you are upgrading via the helm chart

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.3.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.

```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.3.11
```

## 5. Wait for the upgrade to complete

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, MDSes and RGWs are terminated and
replaced with updated versions in sequence. The cluster may be offline very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the MDSes are upgrading. This is normal.
Continue on to the next upgrade step while the update is commencing.

Before moving on, the Ceph cluster's core (RADOS) components (i.e., mons, mgrs, and OSDs) must be
fully updated.

```sh
watch --exec kubectl -n $ROOK_NAMESPACE get deployments -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs from v1.2 to v1.3. When all
deployments report `1/1/1` availability and `rook-version=v1.3.11`, the Ceph cluster's core
components are fully updated.

```console
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.3.11
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.3.11
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.3.11
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.3.11
rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.3.11
rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.2.7
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.2.7
```

The MDS, NFS, and RGW daemons are the last to update. An easy check to see if the upgrade is totally
finished is to check that there is only one `rook-version` reported across the cluster. It is safe
to proceed with the next step before the MDSes and RGWs are finished updating.

```console
# kubectl -n $ROOK_NAMESPACE get deployment -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.2.7
  rook-version=v1.3.11
This cluster is finished:
  rook-version=v1.3.11
```

## 6. Verify the updated cluster

At this point, your Rook operator should be running version `rook/ceph:v1.3.11`.

Verify the Ceph cluster's health using the [health verification section](#health-verification).


## Ceph Version Upgrades

Rook v1.3 supports Ceph Nautilus 14.2.5 or newer and Ceph Octopus v15.2.0 or newer. These are the
only supported major versions of Ceph.

> **IMPORTANT: When an update is requested, the operator will check Ceph's status, if it is in `HEALTH_ERR` it will refuse to do the upgrade.**

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been
updated in the CR), Rook will go through all the daemons one by one and will individually perform
checks on them. It will make sure a particular daemon can be stopped before performing the upgrade,
once the deployment has been updated, it checks if this is ok to continue. After each daemon is
updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for
MDSs, etc.), then only when the condition is met we move to the next daemon. We repeat this process
until all the daemons have been updated.

### Ceph images

Official Ceph container images can be found on [Docker Hub](https://hub.docker.com/r/ceph/ceph/tags/).
These images are tagged in a few ways:

* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v15.2.0-20200324`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v15`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

### Example upgrade to Ceph Octopus

#### 1. Update the main Ceph daemons

The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
Ceph image field in the cluster CRD (`spec:cephVersion:image`).

```sh
NEW_CEPH_IMAGE='ceph/ceph:v15.2.0-20200324'
CLUSTER_NAME="$ROOK_NAMESPACE"  # change if your cluster name is not the Rook namespace
kubectl -n $ROOK_NAMESPACE patch CephCluster $CLUSTER_NAME --type=merge -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

#### 2. Wait for the daemon pod updates to complete

As with upgrading Rook, you must now wait for the upgrade to complete. Status can be determined in a
similar way to the Rook upgrade as well.

```sh
watch --exec kubectl -n $ROOK_NAMESPACE get deployments -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \tceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}'
```

Determining when the Ceph has fully updated is rather simple.

```console
# kubectl -n $ROOK_NAMESPACE get deployment -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{"ceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
    ceph-version=14.2.7-0
    ceph-version=15.2.0-0
This cluster is finished:
    ceph-version=15.2.0-0
```

#### 3. Verify the updated cluster

Verify the Ceph cluster's health using the [health verification section](#health-verification).

If you see a health warning about enabling msgr2, please see the section in the Rook v1.0 guide on
[updating the mon ports](https://rook.io/docs/rook/v1.0/ceph-upgrade.html#6-update-the-mon-ports).

```sh
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph status
```
```console
  cluster:
    id:     b02807da-986a-40b0-ab7a-fa57582b1e4f
    health: HEALTH_WARN
            3 monitors have not enabled msgr2
```

Alternatively, this warning can suppressed if a temporary workaround is needed.

```sh
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph config set global mon_warn_on_msgr2_not_enabled false
```


## CSI Updates
If you have a cluster running with CSI drivers enabled and you have configured the Rook-Ceph
Operator to use custom CSI images, the environment (`env`) variables need to be updated
periodically. If this is the case, it is easiest to `kubectl edit` the Operator Deployment and
modify everything needed at once. You can switch between using Rook-Ceph's default CSI images and
custom CSI images as you wish.

### Use default images
If you would like Rook to use the inbuilt default upstream images, then you may simply remove all
`env` variables matching `ROOK_CSI_*_IMAGE` from the Rook-Ceph operator deployment.

### Use custom images
OR, if you would like to use images hosted in a different location like a local image registry, then
the following `env` variables will need to be configured in the Rook-Ceph operator deployment. The
suggested upstream images are included below, which you should change to match where your images are
located.

```yaml
  env:
  - name: ROOK_CSI_CEPH_IMAGE
    value: "quay.io/cephcsi/cephcsi:v2.1.2"
  - name: ROOK_CSI_REGISTRAR_IMAGE
    value: "quay.io/k8scsi/csi-node-driver-registrar:v1.2.0"
  - name: ROOK_CSI_PROVISIONER_IMAGE
    value: "quay.io/k8scsi/csi-provisioner:v1.4.0"
  - name: ROOK_CSI_SNAPSHOTTER_IMAGE
    value: "quay.io/k8scsi/csi-snapshotter:v1.2.2"
  - name: ROOK_CSI_ATTACHER_IMAGE
    value: "quay.io/k8scsi/csi-attacher:v2.1.0"
  - name: ROOK_CSI_RESIZER_IMAGE
    value: "quay.io/k8scsi/csi-resizer:v0.4.0"
```

### Verifying updates
You can use the below command to see the CSI images currently being used in the cluster. Once there
is only a single version of each image and the version is the latest one you expect, the CSI pods
are updated.

```console
# kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app in (csi-rbdplugin,csi-rbdplugin-provisioner,csi-cephfsplugin,csi-cephfsplugin-provisioner)' | sort | uniq
quay.io/cephcsi/cephcsi:v2.1.2
quay.io/k8scsi/csi-attacher:v2.1.0
quay.io/k8scsi/csi-node-driver-registrar:v1.2.0
quay.io/k8scsi/csi-provisioner:v1.4.0
quay.io/k8scsi/csi-resizer:v0.4.0
quay.io/k8scsi/csi-snapshotter:v1.2.2
quay.io/k8scsi/csi-resizer:v0.4.0
```
