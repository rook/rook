---
title: Upgrades
weight: 60
---

# Upgrades
This guide will walk you through the manual steps to upgrade the software in a Rook cluster from one version to the next.
Rook is a distributed software system and therefore there are multiple components to individually upgrade in the sequence defined in this guide.
After each component is upgraded, it is important to verify that the cluster returns to a healthy and fully functional state.

This guide is just the beginning of upgrade support in Rook.
The goal is to provide prescriptive guidance and knowledge on how to upgrade a live Rook cluster and we hope to get valuable feedback from the community that will be incorporated into an automated upgrade solution by the Rook operator.

We welcome feedback and opening issues!

## Supported Versions
The supported version for this upgrade guide is **from a 0.7 release to a 0.8 release**.
Build-to-build upgrades are not guaranteed to work.
This guide is to test upgrades only between the official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for those releases.
- [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
- [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)

### Patch Release Upgrades
The changes in patch releases are scoped to the minimal changes necessary and are expected to be straight forward to upgrade.
For upgrades of 0.8 patch releases such as `0.8.0` to `0.8.1`, see the [patch release upgrade guide](upgrade-patch.md).

## Considerations
With this manual upgrade guide, there are a few notes to consider:
* **WARNING:** Upgrading a Rook cluster is a manual process in its very early stages.  There may be unexpected issues or obstacles that damage the integrity and health of your storage cluster, including data loss.  Only proceed with this guide if you are comfortable with that.
* This guide assumes that your Rook operator and its agents are running in the `rook-system` namespace. It also assumes that your Rook cluster is in the `rook` namespace.  If any of these components is in a different namespace, search/replace all instances of `-n rook-system` and `-n rook` in this guide with `-n <your namespace>`.
  * New Ceph specific namespaces (`rook-ceph-system` and `rook-ceph`) are now used by default in the new release, but this guide maintains the usage of `rook-system` and `rook` for backwards compatibility.  Note that all user guides and examples have been updated to the new namespaces, so you will need to tweak them to maintain compatibility with the legacy `rook-system` and `rook` namespaces.

## Prerequisites
In order to successfully upgrade a Rook cluster, the following prerequisites must be met:
* The cluster should be in a healthy state with full functionality.
Review the [health verification section](#health-verification) in order to verify your cluster is in a good starting state.
* `dataDirHostPath` must be set in your Cluster spec.
This persists metadata on host nodes, enabling pods to be terminated during the upgrade and for new pods to be created in their place.
More details about `dataDirHostPath` can be found in the [Cluster CRD readme](./ceph-cluster-crd.md#cluster-settings).
* All pods consuming Rook storage should be created, running, and in a steady state.  No Rook persistent volumes should be in the act of being created or deleted.

The minimal sample v0.7 Cluster spec that will be used in this guide can be found below (note that the specific configuration may not be applicable to all environments):
```
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook
  namespace: rook
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
Before we begin the upgrade process, let's first review some ways that you can verify the health of your cluster, ensuring that the upgrade is going smoothly after each step.
Most of the health verification checks for your cluster during the upgrade process can be performed with the Rook toolbox.
For more information about how to run the toolbox, please visit the [Rook toolbox readme](./toolbox.md#running-the-toolbox-in-kubernetes).

### Pods all Running
In a healthy Rook cluster, the operator, the agents and all Rook namespace pods should be in the `Running` state and have few, if any, pod restarts.
To verify this, run the following commands:
```bash
kubectl -n rook-system get pods
kubectl -n rook get pod
```
If pods aren't running or are restarting due to crashes, you can get more information with `kubectl describe pod` and `kubectl logs` for the affected pods.

### Status Output
The Rook toolbox contains the Ceph tools that can give you status details of the cluster with the `ceph status` command.
Let's look at some sample output and review some of the details:
```bash
> kubectl -n rook exec -it rook-ceph-tools -- ceph status
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
* Cluster health: The overall cluster status is `HEALTH_OK` and there are no warning or error status messages displayed.
* Monitors (mon):  All of the monitors are included in the `quorum` list.
* OSDs (osd): All OSDs are `up` and `in`.
* Manager (mgr): The Ceph manager is in the `active` state.
* Placement groups (pgs): All PGs are in the `active+clean` state.

If your `ceph status` output has deviations from the general good health described above, there may be an issue that needs to be investigated further. There are other commands you may run for more details on the health of the system, such as `ceph osd status`.

### Pod Version
The version of a specific pod in the Rook cluster can be verified in its pod spec output.  For example, for the monitor pod `mon0`, we can verify the version it is running with the below commands:
```bash
MON0_POD_NAME=$(kubectl -n rook get pod -l mon=rook-ceph-mon0 -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook get pod ${MON0_POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

### All Pods Status and Version
The status and version of all Rook pods can be collected all at once with the following commands:
```bash
kubectl -n rook-system get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
kubectl -n rook get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
```

### Rook Volume Health
Any pod that is using a Rook volume should also remain healthy:
* The pod should be in the `Running` state with no restarts
* There shouldn't be any errors in its logs
* The pod should still be able to read and write to the attached Rook volume.

## Upgrade Process
The general flow of the upgrade process will be to upgrade the version of a Rook pod, verify the pod is running with the new version, then verify that the overall cluster health is still in a good state.

In this guide, we will be upgrading a live Rook cluster running `v0.7.1` to the next available version of `v0.8`.

Let's get started!

### Upgrading a Build From Master

It is **strongly recommended** that you use [official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the master branch are subject to changes and incompatibilities that will not be supported in the official releases.
Builds from the master branch can have functionality changed and even removed at any time without compatibility support and without prior notice.

If you have a cluster that is running a master build, please see the [appendix for special steps to manually upgrade](#appendix-upgrading-a-build-from-master) the `ceph.rook.io` CRDs in your cluster.


### Agents
The Rook agents are deployed by the operator to run on every node.
They are in charge of handling all operations related to the consumption of storage from the cluster.
The agents are deployed and managed by a Kubernetes daemonset.
Since the agents are stateless, the simplest way to update them is by deleting them and allowing the operator to create them again.

Delete the agent daemonset and permissions:
```bash
kubectl -n rook-system delete daemonset rook-agent
kubectl delete clusterroles rook-agent
kubectl delete clusterrolebindings rook-agent
```

Now when the operator is recreated, the agent daemonset will automatically be created again with the new version.

### Operator Access to Clusters
No longer is the operator given privileges across every namespace. The operator will need privileges to manage each Ceph cluster that is configured. 
The following service account, role, and role bindings are included in `cluster.yaml` when creating a new cluster. Save the following yaml as `cluster-privs.yaml`, 
modifying the `namespace:` attribute of each resource if you are not using the default `rook` or `rook-system` namespaces.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cluster
  namespace: rook
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: rook
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: rook
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-system
---
# Allow the pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: rook
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-cluster
  namespace: rook
```

Now create the objects in the cluster:
```bash
kubectl create -f cluster-privs.yaml
```

### Operator
The Rook operator is the management brains of the cluster, so it should be upgraded first before other components.
In the event that the new version requires a migration of metadata or config, the operator is the one that would understand how to perform that migration.

Since the upgrade process for this version includes support for storage providers beyond Ceph, we will need to start up a Ceph specific operator.
Let's delete the deployment for the old operator and its permissions first:
```bash
kubectl -n rook-system delete deployment rook-operator
kubectl delete clusterroles rook-operator
kubectl delete clusterrolebindings rook-operator
```

Now we need to create the new Ceph specific operator.

**IMPORTANT:** Ensure that you are using the latest manifests from the `release-0.8` branch.  If you have custom configuration options set in your old `rook-operator.yaml` manifest, you will need to set those values in the new Ceph operator manifest below.

Navigate to the new Ceph manifests directory, apply your custom configuration options if you are using any, and then create the new Ceph operator with the command below.
Note that the new operator by default uses by `rook-ceph-system` namespace, but we will use `sed` to edit it in place to use `rook-system` instead for backwards compatibility with your existing cluster.
```bash
cd cluster/examples/kubernetes/ceph
cat operator.yaml | sed -e 's/namespace: rook-ceph-system/namespace: rook-system/g' | kubectl create -f -
```

After the operator starts, after several minutes you may see some new OSD pods being started and then crash looping. This is expected. This will be resolved when you get to the 
[OSD section](#object-storage-daemons-osds).

#### Operator Health Verification
To verify the operator pod is `Running` and using the new version of `rook/ceph:v0.8.0`, use the following commands:
```bash
OPERATOR_POD_NAME=$(kubectl -n rook-system get pods -l app=rook-ceph-operator -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook-system get pod ${OPERATOR_POD_NAME} -o jsonpath='{.status.phase}{"\n"}{.spec.containers[0].image}{"\n"}'
```

Once you've verified the operator is `Running` and on the new version, verify the health of the cluster is still OK.
Instructions for verifying cluster health can be found in the [health verification section](#health-verification).

#### Possible Issue: PGs unknown
After upgrading the operator, the placement groups may show as status unknown. If you see this, go to the section
on [upgrading OSDs](#object-storage-daemons-osds). Upgrading the OSDs will resolve this issue.
```
kubectl -n rook exec -it rook-tools -- ceph status
...
    pgs:     100.000% pgs unknown
             100 unknown
```

### Toolbox
The toolbox pod runs the tools we will use during the upgrade for cluster status. The toolbox is not expected to contain any state,
so we will delete the old pod and start the new toolbox.
```bash
kubectl -n rook delete pod rook-tools
```
After verifying the old tools pod has terminated, start the new toolbox.
You will need to either create the toolbox using the yaml in the `release-0.8` branch or simply set the version of the container to `rook/ceph-toolbox:v0.8.0` before creating the toolbox.
Note the below command uses `sed` to change the new default namespace for the toolbox from `rook-ceph` to `rook` to be backwards compatible with your existing cluster.
```
cat toolbox.yaml | sed -e 's/namespace: rook-ceph/namespace: rook/g' | kubectl create -f -
```

### API
The Rook API service has been removed. Delete the service and its deployment with the following commands:
```bash
kubectl -n rook delete svc rook-api
kubectl -n rook delete deploy rook-api
```

### Monitors
There are multiple monitor pods to upgrade and they are each individually managed by their own replica set.
**For each** monitor's replica set, you will need to update the pod template spec's image version field to `rook/ceph:v0.8.0`.
For example, we can update the replica set for `mon0` with:
```bash
kubectl -n rook set image replicaset/rook-ceph-mon0 rook-ceph-mon=rook/ceph:v0.8.0
```

Once the replica set has been updated, we need to manually terminate the old pod which will trigger the replica set to create a new pod using the new version.
```bash
kubectl -n rook delete pod -l mon=rook-ceph-mon0
```

After the new monitor pod comes up, we can verify that it's in the `Running` state and on the new version:
```bash
kubectl -n rook get pod -l mon=rook-ceph-mon0 -o jsonpath='{.items[0].status.phase}{"\n"}{.items[0].spec.containers[0].image}{"\n"}'
```

At this point, it's very important to ensure that all monitors are `OK` and `in quorum`.
Refer to the [status output section](#status-output) for instructions.
If all of the monitors (and the cluster health overall) look good, then we can move on and repeat the same upgrade steps for the next monitor until all are completed.

**NOTE:** It is possible while upgrading your monitor pods that the operator will find them out of quorum and immediately replace them with a new monitor, such as `mon0` getting replaced by `mon3`.
This is okay as long as the cluster health looks good and all monitors eventually reach quorum again.

### Object Storage Daemons (OSDs)
The OSDs have gone through major changes in the 0.8. While the upgrade steps will seem very disruptive, we feel confident that this will keep your cluster running.
The critical changes to the OSDs include (see also the [design doc](/design/dedicated-osd-pod.md)):
- Each OSD will run in its own pod. No longer will a DaemonSet be deployed to run OSDs on all nodes, or ReplicaSets to run all the OSDs on individual nodes. 
- Each OSD is manged by a K8s Deployment
- A new "discovery" DaemonSet is running in the rook system namespace that will identify all the available devices in the cluster
- The operator will analyze the available devices, provision the desired OSDs with a "prepare" pod on each node where devices will run, then start the deployment for each OSD

When the operator starts, it will automatically detect all the OSDs that had been configured previously and start the OSD pods with the new design.
These new pods will crash loop until you delete the DaemonSet or ReplicaSets from the 0.7 release as follows. After they are deleted, the OSD pods should then start
up with the same configuration and your data will be preserved.

To move to this new design, you will need to delete the previous DaemonSets and ReplicaSets.
```bash
# If you are using the DaemonSet (useAllNodes: true)
kubectl -n rook delete daemonset rook-ceph-osd

# If you are using the ReplicaSets (useAllNodes: false), you will need to delete the replicaset for each node
kubectl -n rook delete replicaset rook-ceph-osd-<node>
```

Now that the 0.7 OSD pods have been deleted, soon you should see the new 0.8 OSD pods in the `Running` state. It may take a few minutes until Kubernetes retries starting the pods.
Once they have all been started, you will see the pods running. If they do not start in a timely manner, you can delete the pods and their K8s deployment will immediately create new pods to replace them.
```
$ kubectl -n rook get pod -l app=rook-ceph-osd
NAME                                  READY     STATUS    RESTARTS   AGE
rook-ceph-osd-id-0-5675d6f5f8-r5b2g   1/1       Running   6          6m
rook-ceph-osd-id-1-69cc6bd8f6-59tcn   1/1       Running   6          6m
rook-ceph-osd-id-2-74b7cf67c5-mtl92   1/1       Running   6          6m
rook-ceph-osd-id-3-757b845567-bk259   1/1       Running   6          6m
rook-ceph-osd-id-4-6cccb5f7d8-wxl2w   1/1       Running   6          6m
rook-ceph-osd-id-5-5b8598cc9f-2pnfb   1/1       Running   6          6m
```

### Ceph Manager
The ceph manager has been renamed in 0.8. The new manager will be started automatically by the operator. 
The old manager and its secret can simply be deleted.

To delete the 0.7 manager, run the following:
```bash
kubectl -n rook delete deploy rook-ceph-mgr0
kubectl -n rook delete secret rook-ceph-mgr0
```

### Legacy Custom Resource Definitions (CRDs)

During this upgrade process, the new Ceph operator automatically migrated legacy custom resources to their new `rook.io/v1alpha2` and `ceph.rook.io/v1beta1` types.
First confirm that there are no remaining legacy CRD instances:

```bash
kubectl -n rook get clusters.rook.io
kubectl -n rook get objectstores.rook.io
kubectl -n rook get filesystems.rook.io
kubectl -n rook get pools.rook.io
kubectl -n rook get volumeattachments.rook.io
```

After confirming that each of those commands returns `No resources found`, it is safe to go ahead and delete the legacy CRD types:

```bash
kubectl delete crd clusters.rook.io
kubectl delete crd filesystems.rook.io
kubectl delete crd objectstores.rook.io
kubectl delete crd pools.rook.io
kubectl delete crd volumeattachments.rook.io
```

After the legacy CRDs are deleted, you will see some errors in the operator log since the operator was trying to watch the legacy CRDs.
To run with a clean log, restart the operator again by deleting the pod.
```bash
kubectl -n rook-system delete pod -l app=rook-ceph-operator
```

### Optional Components
If you have optionally installed either [object storage](./object.md) or a [shared file system](./filesystem.md) in your Rook cluster, the sections below will provide guidance on how to update them as well.
They are both managed by deployments, which we have already covered in this guide, so the instructions will be brief.

#### Object Storage (RGW)
If you have object storage installed, first edit the RGW deployment to use the new image version of `rook/ceph:v0.8.0`:
```bash
kubectl -n rook set image deploy/rook-ceph-rgw-my-store rook-ceph-rgw-my-store=rook/ceph:v0.8.0
```

To verify that the RGW pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-rgw -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

#### Shared File System (MDS)
If you have a shared file system installed, first edit the MDS deployment to use the new image version of `rook/ceph:v0.8.0`:
```bash
kubectl -n rook set image deploy/rook-ceph-mds-myfs rook-ceph-mds-myfs=rook/ceph:v0.8.0
```

To verify that the MDS pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-mds -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

## Completion
At this point, your Rook cluster should be fully upgraded to running version `rook/ceph:v0.8.0` and the cluster should be healthy according to the steps in the [health verification section](#health-verification).

## Upgrading Kubernetes
Rook cluster installations on Kubernetes prior to version 1.7.x, use [ThirdPartyResource](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-third-party-resource/) that have been deprecated as of 1.7 and removed in 1.8. If upgrading your Kubernetes cluster Rook TPRs have to be migrated to CustomResourceDefinition (CRD) following [Kubernetes documentation](https://kubernetes.io/docs/tasks/access-kubernetes-api/migrate-third-party-resource/). Rook TPRs that require migration during upgrade are:
- Cluster
- Pool
- ObjectStore
- Filesystem
- VolumeAttachment

## Appendix: Upgrading a Build from Master

As previously mentioned, it is not recommended to run builds from master since they can change and be otherwise incompatible with the official releases.
However, this section will attempt to provide the steps needed to upgrade a master build of Rook to the `v0.8` release.
These steps are provided "as-is" with no guarantees of correctness in all environments.

**NOTE:** Do not perform these commands if you are using official release versions of Rook, these steps are only for **builds from master**.

First, stop and delete the operator, agents and discover pods:

```console
kubectl -n rook-ceph-system delete daemonset rook-ceph-agent
kubectl -n rook-ceph-system delete daemonset rook-discover
kubectl -n rook-ceph-system delete deployment rook-ceph-operator
```

Ensure the cluster finalizer has been removed so the cluster object can be deleted.

```console
kubectl -n rook-ceph patch clusters.ceph.rook.io rook-ceph -p '{"metadata":{"finalizers": []}}' --type=merge
```

Now backup all of the `ceph.rook.io/v1alpha1` CRD instances:

```console
for c in $(kubectl -n rook-ceph get clusters.ceph.rook.io -o jsonpath='{.items[*].metadata.name}'); do kubectl -n rook-ceph get clusters.ceph.rook.io ${c} -o yaml --export > rook-clusters-${c}-backup.yaml; done
for p in $(kubectl -n rook-ceph get pools.ceph.rook.io -o jsonpath='{.items[*].metadata.name}'); do kubectl -n rook-ceph get pools.ceph.rook.io ${p} -o yaml --export > rook-pools-${p}-backup.yaml; done
for f in $(kubectl -n rook-ceph get filesystems.ceph.rook.io -o jsonpath='{.items[*].metadata.name}'); do kubectl -n rook-ceph get filesystems.ceph.rook.io ${f} -o yaml --export > rook-filesystems-${f}-backup.yaml; done
for o in $(kubectl -n rook-ceph get objectstores.ceph.rook.io -o jsonpath='{.items[*].metadata.name}'); do kubectl -n rook-ceph get objectstores.ceph.rook.io ${o} -o yaml --export > rook-objectstores-${o}-backup.yaml; done
```

Since they have all been backed up, let's remove (delete) the instances now:

```console
kubectl -n rook-ceph delete clusters.ceph.rook.io --all --cascade=false
kubectl -n rook-ceph delete pools.ceph.rook.io --all --cascade=false
kubectl -n rook-ceph delete filesystems.ceph.rook.io --all --cascade=false
kubectl -n rook-ceph delete objectstores.ceph.rook.io --all --cascade=false
```

And we also need to delete the CRD types too:

```console
kubectl delete crd clusters.ceph.rook.io
kubectl delete crd filesystems.ceph.rook.io
kubectl delete crd objectstores.ceph.rook.io
kubectl delete crd pools.ceph.rook.io
```

Wait a few seconds to make sure the types have been completely removed, and now start the oeprator back up again:

```console
kubectl create -f operator.yaml
```

After the operator is running, we can restore all the CRD instances as their new `ceph.rook.io/v1beta1` types:

```console
for c in $(ls rook-clusters-*-backup.yaml); do cat ${c} | sed -e 's/ceph.rook.io\/v1alpha1/ceph.rook.io\/v1beta1/g' -e 's/namespace: ""/namespace: rook-ceph/g' | kubectl create -f -; done
for p in $(ls rook-pools-*-backup.yaml); do cat ${p} | sed -e 's/ceph.rook.io\/v1alpha1/ceph.rook.io\/v1beta1/g' -e 's/namespace: ""/namespace: rook-ceph/g' | kubectl create -f -; done
for f in $(ls rook-filesystems-*-backup.yaml); do cat ${f} | sed -e 's/ceph.rook.io\/v1alpha1/ceph.rook.io\/v1beta1/g' -e 's/namespace: ""/namespace: rook-ceph/g' | kubectl create -f -; done
for o in $(ls rook-objectstores-*-backup.yaml); do cat ${o} | sed -e 's/ceph.rook.io\/v1alpha1/ceph.rook.io\/v1beta1/g' -e 's/namespace: ""/namespace: rook-ceph/g' | kubectl create -f -; done
```

The `ceph.rook.io` CRDs should now be upgraded to `v1beta1`, so you can proceed with the rest of the upgrade process in the [operator health verification section](#operator-health-verification).