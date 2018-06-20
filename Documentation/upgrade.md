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
The supported version for this upgrade guide is **from an 0.7 release to the latest builds**. Until 0.8 is released, the latest builds are labeled such as `v0.7.0-27.gbfc8ec6`. Build-to-build upgrades are not guaranteed to work. This guide is to test upgrades only between the official releases.

For a guide to upgrade previous versions of Rook, please refer to the version of documentation for those releases.
- [Upgrade 0.6 to 0.7](https://rook.io/docs/rook/v0.7/upgrade.html)
- [Upgrade 0.5 to 0.6](https://rook.io/docs/rook/v0.6/upgrade.html)

## Considerations
With this manual upgrade guide, there are a few notes to consider:
* **WARNING:** Upgrading a Rook cluster is a manual process in its very early stages.  There may be unexpected issues or obstacles that damage the integrity and health of your storage cluster, including data loss.  Only proceed with this guide if you are comfortable with that.
* Rook is still in an alpha state.  Migrations and general support for breaking changes across versions are not supported or covered in this guide.
* This guide assumes that your Rook operator and its agents are running in the `rook-system` namespace. It also assumes that your Rook cluster is in the `rook` namespace.  If any of these components is in a different namespace, search/replace all instances of `-n rook-system` and `-n rook` in this guide with `-n <your namespace>`.

## Prerequisites
In order to successfully upgrade a Rook cluster, the following prerequisites must be met:
* The cluster should be in a healthy state with full functionality.
Review the [health verification section](#health-verification) in order to verify your cluster is in a good starting state.
* `dataDirHostPath` must be set in your Cluster spec.
This persists metadata on host nodes, enabling pods to be terminated during the upgrade and for new pods to be created in their place.
More details about `dataDirHostPath` can be found in the [Cluster CRD readme](./cluster-crd.md#cluster-settings).
* All pods consuming Rook storage should be created, running, and in a steady state.  No Rook persistent volumes should be in the act of being created or deleted.

The minimal sample Cluster spec that will be used in this guide can be found below (note that the specific configuration may not be applicable to all environments):
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
> kubectl -n rook exec -it rook-tools -- ceph status
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

In this guide, we will be upgrading a live Rook cluster running `v0.7.0` to the next available version of `v0.8`. Until the `v0.8` release is completed, we will instead use the latest `v0.7` tag such as `v0.7.0-27.gbfc8ec6`.

Let's get started!

### Agents
The Rook agents are deployed by the operator to run on every node. They are in charge of handling all operations related to the consumption of storage from the cluster.
The agents are deployed and managed by a Kubernetes daemonset. Since the agents are stateless, the simplest way to update them is by deleting them and allowing the operator
to create them again.

Delete the agent daemonset:
```bash
kubectl -n rook-system delete daemonset rook-agent
```

Now when the operator is updated, the agent daemonset will automatically be created again with the new version.

### Operator
The Rook operator is the management brains of the cluster, so it should be upgraded first before other components.
In the event that the new version requires a migration of metadata or config, the operator is the one that would understand how to perform that migration.

The operator is managed by a Kubernetes deployment, so in order to upgrade the version of the operator pod, we will need to edit the image version of the pod template in the deployment spec.  This can be done with the following command:
```bash
kubectl -n rook-system set image deployment/rook-operator rook-operator=rook/rook:v0.7.0-27.gbfc8ec6
```
Once the command is executed, Kubernetes will begin the flow of the deployment updating the operator pod.

#### Operator Health Verification
To verify the operator pod is `Running` and using the new version of `rook/rook:v0.7.0-27.gbfc8ec6`, use the following commands:
```bash
OPERATOR_POD_NAME=$(kubectl -n rook-system get pods -l app=rook-operator -o jsonpath='{.items[0].metadata.name}')
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
After verifying the old tools pod has terminated, start the new toolbox. You will need to either create the toolbox using the yaml in the master branch
or simply set the version of the container to `rook/rook:v0.7.0-27.gbfc8ec6` before creating the toolbox.
```
kubectl create -f rook-tools.yaml
```

### API
The Rook API service has been removed. Delete the service and its deployment with the following commands:
```bash
kubectl -n rook delete svc rook-api
kubectl -n rook delete deploy rook-api
```

### Monitors
There are multiple monitor pods to upgrade and they are each individually managed by their own replica set.
**For each** monitor's replica set, you will need to update the pod template spec's image version field to `rook/rook:v0.7.0-27.gbfc8ec6`.
For example, we can update the replica set for `mon0` with:
```bash
kubectl -n rook set image replicaset/rook-ceph-mon0 rook-ceph-mon=rook/rook:v0.7.0-27.gbfc8ec6
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
The OSD pods can be managed in two different ways, depending on how you specified your storage configuration in your [Cluster spec](./cluster-crd.md#cluster-settings).  
* **Use all nodes:** all storage nodes in the cluster will be managed by a single daemon set.
Only the one daemon set will need to be edited to update the image version, then each OSD pod will need to be deleted so that a new pod will be created by the daemon set to take its place.
* **Specify individual nodes:** each storage node specified in the cluster spec will be managed by its own individual replica set.
Each of these replica sets will need to be edited to update the image version, then each OSD pod will need to be deleted so its replica set will start a new pod on the new version to replace it.

In this example, we are going to walk through the case where `useAllNodes: true` was set in the cluster spec, so there will be a single daemon set managing all the OSD pods.

Let's update the container version of either the single OSD daemonset or every OSD replicaset (depending on how the OSDs were deployed).
```
# If using a daemonset for all nodes
kubectl -n rook edit daemonset rook-ceph-osd

# If using a replicaset for specific nodes, edit each one by one
kubectl -n rook edit replicaset rook-ceph-osd-<node>
```

Update the version of the container.
```
        image: rook/rook:v0.7.0-27.gbfc8ec6
```

Once the daemon set (or replica set) is updated, we can begin deleting each OSD pod **one at a time** and verifying a new one comes up to replace it that is running the new version.
After each pod, the cluster health and OSD status should remain or return to an okay state as described in the [health verification section](#health-verification).
To get the names of all the OSD pods, the following can be used:
```bash
kubectl -n rook get pod -l app=rook-ceph-osd -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'
```

Below is an example of deleting just one of the OSD pods (note that the names of your OSD pods will be different):
```bash
kubectl -n rook delete pod rook-ceph-osd-kcj8f
```

The status and version for all OSD pods can be collected with the following command:
```bash
kubectl -n rook get pod -l app=rook-ceph-osd -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

Remember after each OSD pod to verify the cluster health using the instructions found in the [health verification section](#health-verification).

### Ceph Manager
Similar to the Rook operator, the Ceph manager pods are managed by a deployment.
We will edit the deployment to use the new image version of `rook/rook:v0.7.0-27.gbfc8ec6`:
```bash
kubectl -n rook set image deploy/rook-ceph-mgr0 rook-ceph-mgr0=rook/rook:v0.7.0-27.gbfc8ec6
```

To verify that the manager pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-mgr -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

### Optional Components
If you have optionally installed either [object storage](./object.md) or a [shared file system](./filesystem.md) in your Rook cluster, the sections below will provide guidance on how to update them as well.
They are both managed by deployments, which we have already covered in this guide, so the instructions will be brief.

#### Object Storage (RGW)
If you have object storage installed, first edit the RGW deployment to use the new image version of `rook/rook:v0.7.0-27.gbfc8ec6`:
```bash
kubectl -n rook set image deploy/rook-ceph-rgw-my-store rook-ceph-rgw-my-store=rook/rook:v0.7.0-27.gbfc8ec6
```

To verify that the RGW pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-rgw -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

#### Shared File System (MDS)
If you have a shared file system installed, first edit the MDS deployment to use the new image version of `rook/rook:v0.7.0-27.gbfc8ec6`:
```bash
kubectl -n rook set image deploy/rook-ceph-mds-myfs rook-ceph-mds-myfs=rook/rook:v0.7.0-27.gbfc8ec6
```

To verify that the MDS pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-mds -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

## Completion
At this point, your Rook cluster should be fully upgraded to running version `rook/rook:v0.7.0-27.gbfc8ec6` and the cluster should be healthy according to the steps in the [health verification section](#health-verification).

## Upgrading Kubernetes
Rook cluster installations on Kubernetes prior to version 1.7.x, use [ThirdPartyResource](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-third-party-resource/) that have been deprecated as of 1.7 and removed in 1.8. If upgrading your Kubernetes cluster Rook TPRs have to be migrated to CustomResourceDefinition (CRD) following [Kubernetes documentation](https://kubernetes.io/docs/tasks/access-kubernetes-api/migrate-third-party-resource/). Rook TPRs that require migration during upgrade are:
- Cluster
- Pool
- ObjectStore
- Filesystem
- VolumeAttachment
