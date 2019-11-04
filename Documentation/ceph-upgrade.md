---
title: Upgrades
weight: 3800
indent: true
---

# Rook-Ceph Upgrades

This guide will walk you through the steps to upgrade the software in a Rook-Ceph cluster from one
version to the next. This includes both the Rook-Ceph operator software itself as well as the Ceph
cluster software.

With the release of Rook 1.0, upgrades for both the operator and for Ceph are nearly entirely
automated save for where Rook's permissions need to be explicitly updated by an admin. Achieving the
level of upgrade automation has been refined by community feedback, and we will always be open to
further feedback for improving automation and improving Rook.

We welcome feedback and opening issues!

## Supported Versions

Please refer to the upgrade guides from previous releases for supported upgrade paths.
Rook upgrades are only supported between official releases. Upgrades to and from `master` are not
supported.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.

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

## Upgrading the Rook-Ceph Operator

## Patch Release Upgrades

Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the image of the Rook operator. For example, when Rook v1.1.1 is
released, the process of updating from v1.1.0 is as simple as running the following:

```console
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.1.1
```

## Upgrading from v1.0 to v1.1

**Rook releases from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed and even removed at
any time without compatibility support and without prior notice.

**Users are required to upgrade Ceph to Mimic (v13.2.4 or newer) or Nautilus (v14.2.x) now.** Rook 1.0
was the last Rook release which will support Ceph's Luminous (v12.x.x) version. These are the only
supported major versions of Ceph.

Rook documentation for 1.1 has identified some Ceph configuration options that the user is
advised to consider regarding PG management for pools. See the topic [here](ceph-configuration.md#default-pg-and-pgp-counts).
While this is not directly related to the upgrade, it could be beneficial to consider these
options now. If the user determines that these configuration options apply to them, they will be
able to set the configuration as documented once the Rook operator has been upgraded.

## Prerequisites

We will do all our work in the Ceph example manifests directory.

```console
cd $YOUR_ROOK_REPO/cluster/examples/kubernetes/ceph/
```

Unless your Rook cluster was created with customized namespaces, namespaces for Rook clusters
created before v0.8 are likely to be:

* Clusters created by v0.7 or earlier: `rook-system` and `rook`
* Clusters created in v0.8 or v0.9: `rook-ceph-system` and `rook-ceph`
* Clusters created in v1.0 or newer: only `rook-ceph`

With this guide, we do our best not to assume the namespaces in your cluster.
To make things as easy as possible, modify and use the below snippet to configure your environment.
We will use these environment variables throughout this document.

```console
# Parameterize the environment
export ROOK_SYSTEM_NAMESPACE="rook-ceph"
export ROOK_NAMESPACE="rook-ceph"
```

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:

* The cluster must be running Ceph Mimic (v13.2.3 or newer) or Nautilus (v14.2.x) before upgrading
  to Rook 1.1; Ceph Luminous (v12.x.x) is no longer supported.
* The cluster should be in a healthy state with full functionality.
  Review the [health verification section](#health-verification) in order to verify your cluster is
  in a good starting state.
* All pods consuming Rook storage should be created, running, and in a steady state. No Rook
  persistent volumes should be in the act of being created or deleted.

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

```console
kubectl -n $ROOK_SYSTEM_NAMESPACE get pods
kubectl -n $ROOK_NAMESPACE get pods
```

### Status Output

The Rook toolbox contains the Ceph tools that can give you status details of the cluster with the
`ceph status` command. Let's look at an output sample and review some of the details:

```console
$ TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
$ kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph status
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

```console
POD_NAME=$(kubectl -n $ROOK_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon-b)
kubectl -n $ROOK_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

### All Pods Status and Version

The status and container versions for all Rook pods can be collected all at once with the following
commands:

```console
kubectl -n $ROOK_SYSTEM_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}' && \
kubectl -n $ROOK_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0].image}{"\n"}{end}'
```

The `rook-version` label exists on Ceph controller resources. For various resource controllers, a
summary of the resource controllers can be gained with the commands below. These will report the
requested, updated, and currently available replicas for various Rook-Ceph resources in addition to
the version of Rook for resources managed by the updated Rook-Ceph operator. Note that the operator
and toolbox deployments do not have a `rook-version` label set.

```console
kubectl -n $ROOK_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_NAMESPACE get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"  \tsucceeded: "}{.status.succeeded}{"      \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

### Rook Volume Health

Any pod that is using a Rook volume should also remain healthy:

* The pod should be in the `Running` state with few, if any, restarts
* There should be no errors in its logs
* The pod should still be able to read and write to the attached Rook volume.

## Rook Operator Upgrade Process

In the examples given in this guide, we will be upgrading a live Rook cluster running `v1.0.6` to
the version `v1.1.0`. This upgrade should work from any official patch release of Rook 1.0 to any
official patch release of 1.1. We will further assume that your previous cluster was created using
an earlier version of this guide and manifests. If you have created custom manifests, these steps
may not work as written.

**Rook release from `master` are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook. Unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed or removed at any
time without compatibility support and without prior notice.

Let's get started!

### 1. Update modified permissions

> **IMPORTANT**: Ensure that you are using the latest manifests from the `release-1.1` branch. If you
> have custom configuration options set in your 1.0 manifests, you will need to also alter those
> values in the 1.1 manifests.

A few permissions have been added in v1.1. To make updating these resources easy, special upgrade
manifests have been created.

Replace the namespace names in the new resources:

```console
sed "s/ROOK_SYSTEM_NAMESPACE/$ROOK_SYSTEM_NAMESPACE/g" upgrade-from-v1.0-create.yaml > upgrade-from-v1.0-create.yaml.tmp
sed "s/ROOK_NAMESPACE/$ROOK_NAMESPACE/g" upgrade-from-v1.0-create.yaml.tmp > upgrade-from-v1.0-create.yaml
rm -f upgrade-from-v1.0-create.yaml.tmp
sed -i "s/ROOK_SYSTEM_NAMESPACE/$ROOK_SYSTEM_NAMESPACE/g" upgrade-from-v1.0-apply.yaml
```

If you have a v1.0 cluster running with CSI drivers enabled, delete the rbac rules created for CSI

```console
kubectl -n $ROOK_SYSTEM_NAMESPACE delete clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner-rules
kubectl -n $ROOK_SYSTEM_NAMESPACE delete clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner-rules
```

Apply the new permissions:

```console
kubectl create -f upgrade-from-v1.0-create.yaml
kubectl apply -f upgrade-from-v1.0-apply.yaml
```

> **NOTE**: Ignore `Error from server (AlreadyExists)` error while running `kubectl create -f upgrade-from-v1.0-create.yaml`

### 2. Update CSI Driver settings (if applicable)

**If you did not configure the CSI driver in the v1.0 release, skip to step 3.**

If you have a v1.0 cluster running with CSI drivers enabled, the environment (`env`) variables
controlling which Ceph CSI images are used likely need to be updated as well. If this is the case,
it is easiest to `kubectl edit` the operator deployment and modify everything needed at once.

If you would like to use the upstream images which Rook uses by default, then you may simply remove
all `env` variables with the `ROOK_CSI_` prefix from the `CephCluster` resource.

OR, if you would like to use images hosted in a different location like a local image registry, then
the following `env` variables will need to be configured. The suggested upstream images are included
below, which you should change to match where your images are located.

```yaml
  env:
    - name: ROOK_CSI_CEPH_IMAGE
        value: "quay.io/cephcsi/cephcsi:v1.2.1"
    - name: ROOK_CSI_REGISTRAR_IMAGE
        value: "quay.io/k8scsi/csi-node-driver-registrar:v1.1.0"
    - name: ROOK_CSI_PROVISIONER_IMAGE
        value: "quay.io/k8scsi/csi-provisioner:v1.3.0"
    - name: ROOK_CSI_SNAPSHOTTER_IMAGE
        value: "quay.io/k8scsi/csi-snapshotter:v1.2.0"
    - name: ROOK_CSI_ATTACHER_IMAGE
        value: "quay.io/k8scsi/csi-attacher:v1.2.0"
```

You can also remove the `ROOK_CSI_CEPHFS_IMAGE` and `ROOK_CSI_RBD_IMAGE` `env` variables that are
no longer used in Rook.

You should also delete outdated `*.snapshot.storage.io` CRDs that may have been created by the previous version. If they are not cleaned up, there may be an error in preventing the VolumeSnapshots from ever being "Ready-To-Use: true":

```console
kubectl delete crd volumesnapshotclasses.snapshot.storage.k8s.io volumesnapshotcontents.snapshot.storage.k8s.io volumesnapshots.snapshot.storage.k8s.io
```

The new versions of the CRDs will be created when the `csi-rbdplugin-provisioner-0` pod is started following the operator upgrade.

If you have configured the kubelet to use other than `/var/lib/kubelet` please
add below to the operator `env` variables.

```yaml
  env:
    - name: ROOK_CSI_KUBELET_DIR_PATH
      value: "/kubelet/path"
```

At the same time you edit the CSI driver settings, go ahead and update the operator deployment image:

```yaml
  image: rook/ceph:v1.1.0
```

Skip to step 4 since the operator image is already updated.

#### 3. Update the Rook Operator

The largest portion of the upgrade is triggered when the operator's image is updated to `v1.1.x`.
When the operator is updated, it will proceed to update all of the Ceph daemons.
(If step 2 was completed, this change has already been applied.)

```console
kubectl -n $ROOK_SYSTEM_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.1.0
```

### 4. Wait for the upgrade to complete

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, MDSes and RGWs are terminated and
replaced with updated versions in sequence. The cluster may be offline very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the MDSes are upgrading. This is normal.
Continue on to the next upgrade step while the update is commencing.

Before moving on, the Ceph cluster's core (RADOS) components (i.e., mons, mgrs, and OSDs) must be
fully updated.

```console
watch --exec kubectl -n $ROOK_NAMESPACE get deployments -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs from 1.0 to 1.1. When all
deployments report `1/1/1` availability and `rook-version=v1.1.0`, the Ceph cluster's core
components are fully updated.

```console
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-osd-0         req/upd/avl: 1//        rook-version=v1.1.0
rook-ceph-osd-1         req/upd/avl: 1/1/1      rook-version=v1.0.6
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=v1.0.6
```

The MDSes and RGWs are the last daemons to update. An easy check to see if the upgrade is totally
finished is to check that there is only one `rook-version` reported across the cluster. It is safe
to proceed with the next step before the MDSes and RGWs are finished updating.

```console
kubectl -n $ROOK_NAMESPACE get deployment -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
  rook-version=v1.0.6
  rook-version=v1.1.0
This cluster is finished:
  rook-version=v1.1.0
```

### 5. Verify the updated cluster

At this point, your Rook operator should be running version `rook/ceph:v1.1.0`.

Verify the Ceph cluster's health using the [health verification section](#health-verification).

### 6. Delete CSI provisioner and attacher (if applicable)

This has to be done if you have deployed CSI in v1.0 or else you can skip it.

With Rook v1.0 we were deploying cephcsi provisioner (rbd and cephfs) as
statefulset even if the kubernetes version is `>=v1.14.x` with Rook v1.1
cephcsi provisioner (rbd and cephfs) will be deployed as statefulset if the
kubernetes version is `<1.14.x` if not it will be deployed as deployment.

Remove attacher statefulset deployed using Rook 1.0. attacher is now part of
provisioner in Rook 1.1

```console
kubectl delete sts csi-rbdplugin-attacher --namespace $ROOK_NAMESPACE --ignore-not-found
kubectl delete sts csi-cephfs-attacher --namespace $ROOK_NAMESPACE --ignore-not-found
```

If kubernetes version is `>=1.14.x` delete the provisioner statefulset

```console
kubectl delete sts csi-cephfsplugin-provisioner --namespace $ROOK_NAMESPACE
kubectl delete sts csi-rbdplugin-provisioner --namespace $ROOK_NAMESPACE
```

### 7. (Recommended) Migrate config overrides from ConfigMap to Ceph directly

If there are Ceph configuration overrides set in the `config` field of the ConfigMap
`rook-config-override`, it is now possible to migrate those configs manually from the ConfigMap to
Ceph directly as documented [here](ceph-configuration.md#specifying-configuration-options). This is
not required but is recommended because the values configured in Ceph directly can be temporarily
overridden by the user as needed in debug/failure scenarios.

#### Example

List the contents of the override ConfigMap.

```console
$ kubectl --namespace $ROOK_NAMESPACE describe configmap rook-config-override
Name:         rook-config-override
Namespace:    rook-ceph
Labels:       <none>
Annotations:  <none>

Data
====
config:
----
[global]
debug_ms = 1/5

[osd.0]
debug_osd = 10
Events:  <none>
```

Apply the configurations to Ceph directly using Ceph's CLI.

```console
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph config set global debug_ms 1/5
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph config set osd.0 debug_osd 10
```

Empty the config override in the ConfigMap.

```console
kubectl --namespace $ROOK_NAMESPACE patch configmap rook-config-override --type=merge -p '{"data": {"config": ""}}'
```

### 8. (Recommended) Consider required Ceph config settings

We highly recommend updates to your cluster regarding PG management. There is a new setting where
Rook can enable the automatic PG management, or you can continue managing it manually.
See more information in the docs [here](ceph-configuration.md#default-pg-and-pgp-counts).

### 9. Update Rook-Ceph custom resource definitions

> **IMPORTANT**: Do not perform this step until ALL existing Rook-Ceph clusters are updated!

After all Rook-Ceph clusters have been updated following the steps above, update the Rook-Ceph
Custom Resource Definitions. This is important to get the latest schema validations that will
support running external Ceph clusters and will also help with creating or modifying Rook-Ceph
deployments in the future.

```console
kubectl apply -f upgrade-from-v1.0-crds.yaml
```

## Ceph Version Upgrades

Rook 1.1 supports Ceph Mimic v13.2.4 or newer and Ceph Nautilus v14.2.0 or newer. These are the only
supported major versions of Ceph.

> **IMPORTANT: When an update is requested, the operator will check Ceph's status, if it is in `HEALTH_ERR` it will refuse to do the upgrade.**

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been
updated in the CR), Rook will go through all the daemons one by one and will individually perform
checks on them. It will make sure a particular daemon can be stopped before performing the upgrade,
once the deployment has been updated, it checks if this is ok to continue. After each daemon is
updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for
MDSs, etc.), then only when the condition is met we move to the next daemon. We repeat this process
until all the daemons have been updated.

## Ceph images

Official Ceph container images can be found on [Docker Hub](https://hub.docker.com/r/ceph/ceph/tags/).
These images are tagged in a few ways:

* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v13.2.6-20190830`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v13`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

## Example upgrade to Ceph Nautilus

### 1. Update the main Ceph daemons

The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
Ceph image field in the cluster CRD (`spec:cephVersion:image`).

```console
NEW_CEPH_IMAGE='ceph/ceph:v14.2.2-20190830'
CLUSTER_NAME="$ROOK_NAMESPACE"  # change if your cluster name is not the Rook namespace
kubectl -n $ROOK_NAMESPACE patch CephCluster $CLUSTER_NAME --type=merge -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

### 2. Wait for the daemon pod updates to complete

As with upgrading Rook, you must now wait for the upgrade to complete. Status can be determined in a
similar way to the Rook upgrade as well.

```console
watch --exec kubectl -n $ROOK_NAMESPACE get deployments -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \tceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}'
```

Determining when the Ceph has fully updated is rather simple.

```console
kubectl -n $ROOK_NAMESPACE get deployment -l rook_cluster=$ROOK_NAMESPACE -o jsonpath='{range .items[*]}{"ceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
    ceph-version=13.2.6
    ceph-version=14.2.2
This cluster is finished:
    ceph-version=14.2.2
```

### 3. Verify the updated cluster

Verify the Ceph cluster's health using the [health verification section](#health-verification).

If you see a health warning about enabling msgr2, please see the section in the Rook v1.0 guide on
[updating the mon ports](https://rook.io/docs/rook/v1.0/ceph-upgrade.html#6-update-the-mon-ports).

```console
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph status
  cluster:
    id:     b02807da-986a-40b0-ab7a-fa57582b1e4f
    health: HEALTH_WARN
            3 monitors have not enabled msgr2
```

Alternatively, this warning can suppressed if a temporary workaround is needed.

```console
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph config set global mon_warn_on_msgr2_not_enabled false
```
