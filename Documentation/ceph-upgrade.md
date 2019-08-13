---
title: Upgrades
weight: 3800
indent: true
---

# Ceph Upgrades
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
Rook ugrades are only supported between official releases. Upgrades to and from `master` are not supported.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.
- [Upgrade 0.9 to 1.0](https://rook.io/docs/rook/v1.0/ceph-upgrade.html)
- [Upgrade 0.8 to 0.9](https://rook.io/docs/rook/v0.9/ceph-upgrade.html)
- [Upgrade 0.7 to 0.8](https://rook.io/docs/rook/v0.8/upgrade.html)
- [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
- [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)


## Considerations
With this upgrade guide, there are a few notes to consider:
* **WARNING:** Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss. Only
  proceed with this guide if you are comfortable with that.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
  both Rook operator updates and for Ceph version updates.
* We recommend that you read this document in full before you undertake a Rook cluster upgrade.


# Upgrading the Rook-Ceph Operator

## Patch Release Upgrades
Unless otherwise noted due to extenuating requirements, upgrades from one patch release of Rook to
another are as simple as updating the image of the Rook operator. For example, when Rook v1.0.1 is
released, the process of updating from v1.0.0 is as simple as running the following:
```
kubectl -n rook-ceph set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.0.1
```

## Upgrading from v1.0 to the latest

Rook upgrades to master builds are not supported. When v1.1 is released, the upgrade will be supported
from 1.0 to 1.1. Master builds should only be used for test environments.

**Rook releases from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed and even removed at
any time without compatibility support and without prior notice.

## Prerequisites
We will do all our work in the Ceph example manifests directory.
```sh
cd $YOUR_ROOK_REPO/cluster/examples/kubernetes/ceph/
```

Unless your Rook cluster was created with customized namespaces, namespaces for Rook clusters
created before v0.8 are likely to be `rook-system` and `rook`, and for Rook-Ceph clusters created
with v0.8 or after, `rook-ceph-system` and `rook-ceph`. With this guide, we do our best not to
assume the namespaces in your cluster. To make things as easy as possible, modify and use the below
snippet to configure your environment. We will use these environment variables throughout this
document.
```sh
# Parameterize the environment
export ROOK_SYSTEM_NAMESPACE="rook-ceph-system"
export ROOK_NAMESPACE="rook-ceph"
```

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:
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
- [General troubleshooting](./common-issues.md)
- [Ceph troubleshooting](./ceph-common-issues.md)

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
> TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
> kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph status
  cluster:
    id:     fe7ae378-dc77-46a1-801b-de05286aa78e
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum rook-ceph-mon0,rook-ceph-mon1,rook-ceph-mon2
    mgr: rook-ceph-mgr0(active)
    osd: 1 osds: 1 up, 1 in

  data:
    pools:   1 pools, 100 pgs
    objects: 0 objects, 0 bytes
    usage:   2049 MB used, 15466 MB / 17516 MB avail
    pgs:     100 active+clean
```

In the output above, note the following indications that the cluster is in a healthy state:
* Cluster health: The overall cluster status is `HEALTH_OK` and there are no warning or error status
  messages displayed.
* Monitors (mon):  All of the monitors are included in the `quorum` list.
* OSDs (osd): All OSDs are `up` and `in`.
* Manager (mgr): The Ceph manager is in the `active` state.
* Placement groups (pgs): All PGs are in the `active+clean` state.

If your `ceph status` output has deviations from the general good health described above, there may
be an issue that needs to be investigated further. There are other commands you may run for more
details on the health of the system, such as `ceph osd status`.

### Container Versions
The container version running in a specific pod in the Rook cluster can be verified in its pod spec
output. For example for the monitor pod `mon-b`, we can verify the container version it is running
with the below commands:
```bash
POD_NAME=$(kubectl -n $ROOK_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon-b)
kubectl -n $ROOK_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

### All Pods Status and Version
The status and container versions for all Rook pods can be collected all at once with the following
commands:
```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}' && \
kubectl -n $ROOK_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0].image}{"\n"}{end}'
```

Rook 1.0 also introduces the `rook-version` label to Ceph controller resources. For various
resource controllers, a summary of the resource controllers can be gained with the commands below.
These will report the requested, updated, and currently available replicas for various Rook-Ceph
resources in addition to the version of Rook for resources managed by the updated Rook-Ceph operator.
```sh
kubectl -n $ROOK_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_NAMESPACE get daemonsets -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.status.desiredNumberScheduled}{"/"}{.status.updatedNumberScheduled}{"/"}{.status.numberAvailable}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_NAMESPACE get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"  \tsucceeded: "}{.status.succeeded}{"      \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

### Rook Volume Health
Any pod that is using a Rook volume should also remain healthy:
* The pod should be in the `Running` state with few, if any, restarts
* There should be no errors in its logs
* The pod should still be able to read and write to the attached Rook volume.


## Rook Operator Upgrade Process

**This is only a placeholder guide for upgrading from v1.0 to v1.1. This is not final documentation
for the 1.1 release**

In the examples given in this guide, we will be upgrading a live Rook cluster running `v1.0.2` to
the version `v1.1.0`. This upgrade should work from any official patch release of Rook 1.0 to any official
patch release of 1.1. We will further assume that your previous cluster was created using an earlier
version of this guide and manifests. If you have created custom manifests, these steps may not work
as written.

**Rook release from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed and even removed at
any time without compatibility support and without prior notice.

Let's get started!

### 1. Configure manifests
**IMPORTANT:** Ensure that you are using the latest manifests from the `release-1.1` branch. If you
have custom configuration options set in your 1.0 manifests, you will need to also alter those
values in the 1.1 manifests.

If your cluster does not use the `rook-ceph-system` and `rook-ceph` namespaces, you will need to
replace all manifest references to these namespaces with references to those used by your cluster.
We can use a few simple `sed` commands to do this for all manifests at once.
```sh
# Replace yaml file namespaces with sed (and make backups)
sed -i.bak -e "s/namespace: rook-ceph-system/namespace: $ROOK_SYSTEM_NAMESPACE/g" *.yaml
sed -i -e "s/namespace: rook-ceph/namespace: $ROOK_NAMESPACE/g" *.yaml
# Reduce clutter by moving the backups we just created
mkdir backups
mv *.bak backups/
```

### 2. Update modified permissions
A few of permissions have been added or modified in v1.1. To make updating these resources easy,
special upgrade manifests have been created.
```sh
kubectl delete -f upgrade-from-v1.0-delete.yaml
kubectl create -f upgrade-from-v1.0-create.yaml
```
Upgrade notes have been added to `upgrade-from-v1.0-create.yaml` identifying the changes made to 1.1
to aid users in any manifest-related auditing they wish to do.

### 3. Update the Rook operator image
The largest portion of the upgrade is triggered when the operator's image is updated to `v1.1.x`.
```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v1.1.0
```

Watch now in amazement as the Ceph mons, mgrs, OSDs, rbd-mirrors, mdses and rgws are terminated and
replaced with updated versions in sequence. The cluster may be offline very briefly as mons update,
and the Ceph Filesystem may fall offline a few times while the mdses are upgrading. This is normal.
Continue on to the next upgrade step while the update is commencing.

### 4. Wait for the upgrade to complete
Before moving on, the Ceph cluster's core (RADOS) components should be fully updated.

```sh
watch --exec kubectl -n $ROOK_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

As an example, this cluster is midway through updating the OSDs from 1.0 to 0.9. When all
deployments report `1/1/1` availability and `rook-version=v1.1.0`, the Ceph cluster's core
components are fully updated.
```sh
Every 2.0s: kubectl -n rook-ceph get deployment -o j...

rook-ceph-mgr-a         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-a         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-b         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-mon-c         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-osd-0         req/upd/avl: 1/1/1      rook-version=v1.1.0
rook-ceph-osd-1         req/upd/avl: 1//        rook-version=v1.1.0
rook-ceph-osd-2         req/upd/avl: 1/1/1      rook-version=
```

The mdses and rgws are the last daemons to update. An easy check to see if the upgrade is totally
finished is to check that there is only one `rook-version` reported across the cluster. It is safe
to proceed with the next step before the mdses and rgws are finished updating.
```sh
(kubectl -n $ROOK_NAMESPACE get deployment -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}' && kubectl -n $ROOK_NAMESPACE get daemonset -o jsonpath='{range .items[*]}{"rook-version="}{.metadata.labels.rook-version}{"\n"}{end}') | uniq
# This cluster is not yet finished:
#   rook-version=
#   rook-version=v1.1.0
# This cluster is finished:
#   rook-version=v1.1.0
```

### 5. Verify the updated cluster
At this point, your Rook operator should be running version `rook/ceph:v1.1.0`

Verify the Ceph cluster's health using the [health verification section](#health-verification).


# Ceph Version Upgrades
Rook 1.0 was the last Rook release which will support Ceph's Luminous (v12.x.x) version. Users are
required to upgrade to Mimic (v13.2.4 or newer) or Nautilus (v14.2.x) now. Rook 1.1 will only run with
Mimic or newer, though running with Octopus requires the `allowUnsupported: true` flag.

**IMPORTANT: This section only applies to clusters running Rook 1.0 or newer**

**IMPORTANT: When an update is requested, the operator will check Ceph's status, if it is in `HEALTH_ERR` it will refuse to do the upgrade.**

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been updated in the CR), Rook will go through all the daemons one by one and will individually perform checks on them. It will make sure a particular daemon can be stopped before performing the upgrade, once the deployment has been updated, it checks if this is ok to continue. After each daemon is updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for MDSs, etc.), then only when the condition is met we move to the next daemon. We repeat this process until all the daemons have been updated.

## Ceph images
Official Ceph container images can be found on [Docker Hub](https://hub.docker.com/r/ceph/ceph/tags/).
These images are tagged in a few ways:
* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v13.2.2-20181023`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v13`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

## Example upgrade to Ceph Nautilus

### 1. Update the main Ceph daemons
The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
Ceph image field in the cluster CRD (`spec:cephVersion:image`).
```sh
NEW_CEPH_IMAGE='ceph/ceph:v14.2.2-20190722'
CLUSTER_NAME="$ROOK_NAMESPACE"  # change if your cluster name is not the Rook namespace
kubectl -n $ROOK_NAMESPACE patch CephCluster $CLUSTER_NAME --type=merge \
  -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

### 2. Wait for the daemon pod updates to complete
As with upgrading Rook, you must now wait for the upgrade to complete. Determining when the Ceph
version has fully updated is rather simple.
```sh
watch kubectl -n $ROOK_NAMESPACE describe pods | grep "Image:.*ceph/ceph" | sort | uniq
# This cluster is not yet finished:
#      Image:         ceph/ceph:v13.2.2-20181023
#      Image:         ceph/ceph:v14.2.2-20190722
# This cluster is also finished:
#      Image:         ceph/ceph:v14.2.2-20190722
```

### 3. Enable Nautilus features
When upgrading to Nautilus (v14.x.x), a final command should be issued to Ceph to take advantage of
the latest Ceph features. This does not apply for upgrades to Mimic (v13.x.x)

```sh
TOOLS_POD=$(kubectl -n $ROOK_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')
kubectl -n $ROOK_NAMESPACE exec -it $TOOLS_POD -- ceph osd require-osd-release nautilus
```

### 4. Verify the updated cluster
Verify the Ceph cluster's health using the [health verification section](#health-verification).

If you see a health warning about enabling msgr2, please see the section in the 1.0 guide on
[Updating the Mon Ports](https://rook.io/docs/rook/v1.0/ceph-upgrade.html#6-update-the-mon-ports).
```
[root@minikube /]# ceph -s
  cluster:
    id:     b02807da-986a-40b0-ab7a-fa57582b1e4f
    health: HEALTH_WARN
            3 monitors have not enabled msgr2
```

Alternatively, this warning can suppressed if a temporary workaround is needed.
```
ceph config set global mon_warn_on_msgr2_not_enabled false
```
