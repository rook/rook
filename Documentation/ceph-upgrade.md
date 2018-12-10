---
title: Upgrades
weight: 38
indent: true
---

# Ceph Upgrades
This guide will walk you through the manual steps to upgrade the software in a Rook cluster from one
version to the next. Rook is a distributed software system and therefore there are multiple
components to individually upgrade in the sequence defined in this guide. After each component is
upgraded, it is important to verify that the cluster returns to a healthy and fully functional
state.

With the release of Rook 0.9, significant progress has been made toward the goal of incorporating
an automated upgrade solution into the Rook operator. While significant, this has merely lain the
foundation for automated upgrade support, and we will continue to be open to community feedback for
further improving and refining upgrade automation.

We welcome feedback and opening issues!


## Supported Versions
The supported version for this upgrade guide is **from a 0.8 release to a 0.9 release**.
Build-to-build upgrades are not guaranteed to work. This guide is to perform upgrades only between
the official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for
those releases.
- [Upgrade 0.7 to 0.8](https://rook.io/docs/rook/v0.8/upgrade.html)
- [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
- [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)

### Patch Release Upgrades
One of the goals of the 0.9 release is that patch releases are able to be automated completely by
the Rook operator. It is intended that upgrades from one patch release to another are as simple as
updating the image of the Rook operator. For example, when Rook v0.9.1 is released, the process
should be as simple as running the following:
```
kubectl -n rook-ceph-system set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v0.9.x
```


## Considerations
With this upgrade guide, there are a few notes to consider:
* **WARNING:** Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss. Only
  proceed with this guide if you are comfortable with that.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process for
  both Rook operator updates and for Ceph daemon updates.
* Rook is able to handle a great deal of the upgrade on its own, but manual steps are still
  necessary. This is in part to reduce the Kubernetes privileges required by the Rook operator.
* We recommend that you read this document in full before you undertake a Rook cluster upgrade.


## Prerequisites
We will do all our work in the Ceph example manifests directory.
```sh
cd $YOUR_ROOK_REPO/cluster/examples/kubernetes/ceph/
```

Unless your Rook cluster was created with customized namespaces, namespaces for Rook clusters
created before v0.8 are likely to be `rook-system` and `rook`, and for Rook-Ceph clusters created
with v0.8, `rook-ceph-system` and `rook-ceph`. With this guide, we do our best not to assume the
namespaces in your cluster. To make things as easy as possible, modify and use the below snippet to
configure your environment. We will use these environment variables throughout this document.
```sh
# Parameterize the environment
export ROOK_SYSTEM_NAMESPACE="rook-ceph-system"
export ROOK_NAMESPACE="rook-ceph"
```

We should start up the new toolbox pod before moving on, and this can be our first test of the
namespace environment variables.
```sh
kubectl -n $ROOK_NAMESPACE delete pod rook-ceph-tools
kubectl -n $ROOK_NAMESPACE create -f toolbox.yaml
```

In order to successfully upgrade a Rook cluster, the following prerequisites must be met:
* The cluster should be in a healthy state with full functionality.
  Review the [health verification section](#health-verification) in order to verify your cluster is
  in a good starting state.
* All pods consuming Rook storage should be created, running, and in a steady state. No Rook
  persistent volumes should be in the act of being created or deleted.

The minimal sample v0.8 Cluster spec that will be used in this guide can be found below (note that
the specific configuration may not be applicable to all environments):
```
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: ceph.rook.io/v1
kind: CephCluster
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  storage:
    useAllNodes: true
    useAllDevices: true
    storeConfig:
      storeType: bluestore
      databaseSizeMB: 1024
      journalSizeMB: 1024
```


## Health Verification
Before we begin the upgrade process, let's first review some ways that you can verify the health of
your cluster, ensuring that the upgrade is going smoothly after each step. Most of the health
verification checks for your cluster during the upgrade process can be performed with the Rook
toolbox. For more information about how to run the toolbox, please visit the
[Rook toolbox readme](./ceph-toolbox.md#running-the-toolbox-in-kubernetes).

### Pods all Running
In a healthy Rook cluster, the operator, the agents and all Rook namespace pods should be in the
`Running` state and have few, if any, pod restarts. To verify this, run the following commands:
```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE get pods
kubectl -n $ROOK_NAMESPACE get pod
```

If pods aren't running or are restarting due to crashes, you can get more information with
and  for the affected pods by trying the commands below.
* `kubectl -n <namespace> describe pod`
* `kubectl -n <namespace> logs --previous`

All Ceph daemon pods have a `config-init` init container which creates the config files for the
daemon, and some daemons have other, specialized init containers, so you may need to look at logs
for different containers within the pod.
* `kubectl -n <namespace> logs -c <container name>`

### Status Output
The Rook toolbox contains the Ceph tools that can give you status details of the cluster with the
`ceph status` command. Let's look at some sample output and review some of the details:
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

### Pod Version
The version of a specific pod in the Rook cluster can be verified in its pod spec output. For
example for the monitor pod `mon0`, we can verify the version it is running with the below commands:
```bash
POD_NAME=$(kubectl -n $ROOK_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon0)
kubectl -n $ROOK_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

### All Pods Status and Version
The status and version of all Rook pods can be collected all at once with the following commands:
```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}'
kubectl -n $ROOK_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}'
```

### Rook Volume Health
Any pod that is using a Rook volume should also remain healthy:
* The pod should be in the `Running` state with no restarts
* There shouldn't be any errors in its logs
* The pod should still be able to read and write to the attached Rook volume.


## Upgrade process
In this guide, we will be upgrading a live Rook cluster running `v0.8.3` to the next available
version of `v0.9`. We will further assume that your previous cluster was created using an earlier
version of this guide and manifests.

If you have created custom manifests, these steps may not work as written. The git diff between the
`release-0.9` branch and the `release-0.8` branch may give you a better idea of what you need to
change for your from-scratch cluster:
`git diff release-0.8 release-0.9 -- cluster/examples/kubernetes/ceph/`

**Rook release from master are expressly unsupported.** It is strongly recommended that you use
[official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the
master branch are subject to changes and incompatibilities that will not be supported in the
official releases. Builds from the master branch can have functionality changed and even removed at
any time without compatibility support and without prior notice.

Let's get started!

### 1. Configure manifests
**IMPORTANT:** Ensure that you are using the latest manifests from the `release-0.9` branch. If you
have custom configuration options set in your old manifests, you will need to also alter those
values in the v0.9 manifests. It may be notable that the `serviceAccount` property has been removed
from the CephCluster CRD; default values will now be used.

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

### 2. Update modifed/added resources
A number of custom resources have been modified and added in v0.9. To make updating these resources
easy, special upgrade manifest has been created.
```sh
kubectl create -f upgrade-from-v0.8-create.yaml
kubectl replace -f upgrade-from-v0.8-replace.yaml
```

**Pod Security Policies:** If your cluster has pod security policies enabled and you used the RBAC
documentation, you will need to add the new `rook-ceph-osd` service account to the subject of the
`rook-ceph-osd-psp` role binding. Notice here that the new service account exists alongside the old
service account so that the existing OSDs may still function during the upgrade.
```sh
kubectl -n $ROOK_NAMESPACE patch rolebinding rook-ceph-osd-psp -p "{\"subjects\": [ \
  {\"kind\": \"ServiceAccount\", \"name\": \"rook-ceph-cluster\", \"namespace\": \"$ROOK_NAMESPACE\"}, \
  {\"kind\": \"ServiceAccount\", \"name\": \"rook-ceph-osd\", \"namespace\": \"$ROOK_NAMESPACE\"}]}"
```

### 3. Update the Rook operator image
The largest portion of the upgrade is triggered when the operator's image is updated to v0.9.0, and
with the greatly-expanded automatic update features in the new version, this is all done
automatically.
```sh
kubectl -n $ROOK_SYSTEM_NAMESPACE set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v0.9.0
```

Watch now in amazement as the Ceph MONs, MGR, OSDs, RGWs, and MDSes are terminated and replaced with
updated versions in sequence. The cluster may be offline very briefly as MONs update, and the Ceph
Filesystem may fall offline a few times while the MDSes are upgrading. This is normal. Continue on
to the next upgrade step while the update is commencing.

### 4. Delete the old MGR replicaset
After the MONs have updated, the MGR will update, and because of the new MGR service account with
fewer decreased privileges, the old MGR replica set must be deleted manually. Once you see two MGR
replica sets, delete the old one using the Rook v0.8 image.
```sh
kubectl -n $ROOK_NAMESPACE get rs -l app=rook-ceph-mgr \
  -o custom-columns='name:.metadata.name,image:.spec.template.spec.containers[0].image'
# Sample output:
#  rook-ceph-mgr-a-deletethis   rook/ceph:v0.8.3
#  rook-ceph-mgr-a-dontdelete   ceph/ceph:v12.2.9-2018102
kubectl -n $ROOK_NAMESPACE delete rs 'rook-ceph-mgr-a-deletethis'  # edit name to match your output
```

### 5. Wait for the upgrade to complete
Before moving on, the cluster's main components should be fully updated. A telltale indication that
the upgrade is nearly finished is that all the osd pods will have been renamed slightly. An example
of an OSD pod name from v0.8 is `rook-ceph-osd-id-0-6795dff4bb-d7pz9`. This will have changed to
something like `rook-ceph-osd-0-74fcfd9c5b-58gd8` after the upgrade where the `id-` has been dropped
between `osd` and the OSD ID.
```sh
watch -c kubectl -n $ROOK_NAMESPACE get pods
```

The MDSes and RGWs are the last daemons to update. An easy check to see if the upgrade is totally
finished is to check that there is only one version of `rook/ceph` and one version of `ceph/ceph`
being used in the cluster.
```sh
kubectl -n $ROOK_NAMESPACE describe pods | grep "Image:.*" | sort | uniq
# This cluster is not yet finished:
#      Image:         ceph/ceph:v12.2.9-20181026
#      Image:         rook/ceph:v0.9.0
#      Image:         rook/ceph:v0.8.3
# This cluster is finished:
#      Image:         ceph/ceph:v12.2.9-20181026
#      Image:         rook/ceph:v0.9.0
```

### 6. Remove unused resources
Finally, resources present in v0.8 that are no longer used in v0.9 can be safely removed.
```sh
# old osd service account
kubectl -n $ROOK_NAMESPACE delete serviceaccount rook-ceph-cluster
kubectl -n $ROOK_NAMESPACE delete role           rook-ceph-cluster
kubectl -n $ROOK_NAMESPACE delete rolebinding    rook-ceph-cluster
# old CRDs
kubectl -n $ROOK_NAMESPACE delete crd clusters.ceph.rook.io
kubectl -n $ROOK_NAMESPACE delete crd filesystems.ceph.rook.io
kubectl -n $ROOK_NAMESPACE delete crd objectstores.ceph.rook.io
kubectl -n $ROOK_NAMESPACE delete crd pools.ceph.rook.io
```

**Pod Security Policies:** If your cluster uses pod security policies, you can now remove the old
`rook-ceph-cluster` service account from the subject of the `rook-ceph-osd-psp` role binding.
```sh
kubectl -n $ROOK_NAMESPACE patch rolebinding rook-ceph-osd-psp -p "{\"subjects\": [ \
  {\"kind\": \"ServiceAccount\", \"name\": \"rook-ceph-osd\", \"namespace\": \"$ROOK_NAMESPACE\"}]}"
```

### 7. Verify the updated cluster
At this point, your Rook operator should be running version `rook/ceph:v0.9.0`, and the Ceph daemons
should be running image `ceph/ceph:v12.2.9-20181026`. The Rook operator version and the Ceph version
are no longer tied together, and we'll cover how to upgrade Ceph later in this document.

Verify the Ceph cluster's health using the [health verification section](#health-verification).

### 8. Update optional components
In general, ancillary components of Rook can be updated updating the CRD, then replacing the
resource.
```sh
kubectl -n $ROOK_SYSTEM replace -f my-manifest.yaml
```

#### File and object storage
There have been no significant changes to the file or object storage CRDs, so updating these should
be unnecessary.

#### Ceph dashboard
The Ceph dashboard service from v0.8 (`rook-ceph-mgr-dashboard-external`) does not need updated at
this time.

## Ceph Daemon Upgrades
By default, an upgraded Rook cluster will be running Ceph Luminous (v12), the same Ceph version
released with Rook v0.8. Now that the Rook and Ceph versions are independently controllable, you can
choose to update Ceph at any time.

### Ceph images
Official Ceph container images can be found on [Docker Hub](https://hub.docker.com/r/ceph/ceph/tags/).
These images are tagged in a few ways:
* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v13.2.2-20181023`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v13`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images above will not be supported.**

### Example upgrade to Ceph Mimic

#### 1. Update the main Ceph daemons
The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
Ceph image field in the cluster CRD (`spec:cephVersion:image`).
```sh
# sed -i.bak "s%image: .*%image: $NEW_CEPH_IMAGE%" cluster.yaml
# kubectl -n $ROOK_SYSTEM_NAMESPACE replace -f cluster.yaml
NEW_CEPH_IMAGE='ceph/ceph:v13.2.2-20181023'
CLUSTER_NAME="$ROOK_NAMESPACE"  # change if your cluster name is not the Rook namespace
kubectl -n $ROOK_NAMESPACE patch CephCluster $CLUSTER_NAME --type=merge \
  -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

As with upgrading Rook, you must now [wait for the upgrade to complete](#5.-wait-for-the-upgrade-to-complete).
Unlike with the Rook upgrade, there is no at-a-glance sign that the upgrade is complete. We
suggest watching the cluster upgrade carefully, and it is likely safe to assume the upgrade is
complete if there have been no pods terminated in over a minute.
```sh
watch -c kubectl -n $ROOK_NAMESPACE get pods
```

To verify the Ceph upgrade is complete, check that all the images Rook is using are the newest ones.
```sh
kubectl -n $ROOK_NAMESPACE describe pods | grep "Image:.*ceph/ceph" | sort | uniq
# This cluster is not yet finished:
#      Image:         ceph/ceph:v12.2.9-20181026
#      Image:         ceph/ceph:v13.2.2-20181023
#      Image:         rook/ceph:v0.9.0
# This cluster is finished:
#      Image:         ceph/ceph:v13.2.2-20181023
#      Image:         rook/ceph:v0.9.0
```

#### 2. Update dashboard external service if applicable
There have been some changes to the Ceph dashboard in Mimic which affect Rook. In Ceph Luminous
(ceph:v12), the dashboard uses HTTP on port 7000 by default, and the v0.8 dashboard service used
this port. In Ceph Mimic (ceph:v13), the dashboard uses HTTPS on port 8443 by default. If you are
upgrading from Ceph Luminous to Ceph Mimic, you must update the dashboard external service if you
are using it.

To upgrade a dashboard external service created in v0.8, the old service must be deleted and the
appropriate new service started.
added.
```sh
kubectl -n $ROOK_NAMESPACE delete service rook-ceph-mgr-dashboard-external
kubectl -n $ROOK_NAMESPACE create -f dashboard-external-https.yaml  # for Ceph Mimic (v13)
```

For dashboard external services installed in v0.9, the HTTP service name has changed from the above,
and a slight modification to these steps are needed.
```sh
kubectl -n $ROOK_NAMESPACE delete -f service rook-ceph-mgr-dashboard-external-http
kubectl -n $ROOK_NAMESPACE create -f dashboard-external-https.yaml  # for Ceph Mimic (v13)
```
