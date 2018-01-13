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
The Rook toolbox contains the `rookctl` command line tool that can give you status details of the cluster with the `rookctl status` command.
Let's look at some sample output and review some of the details:
```bash
> kubectl -n rook exec -it rook-tools -- rookctl status
OVERALL STATUS: OK

USAGE:
TOTAL       USED        DATA         AVAILABLE
26.62 GiB   12.25 GiB   252.76 MiB   14.37 GiB

MONITORS:
NAME             ADDRESS             IN QUORUM   STATUS
rook-ceph-mon2   10.3.0.15:6790/0    true        OK
rook-ceph-mon0   10.3.0.129:6790/0   true        OK
rook-ceph-mon1   10.3.0.218:6790/0   true        OK

MGRs:
NAME             STATUS
rook-ceph-mgr0   Active

OSDs:
TOTAL     UP        IN        FULL      NEAR FULL
6         6         6         false     false

PLACEMENT GROUPS (900 total):
STATE          COUNT
active+clean   900
```
In the output above, note the following indications that the cluster is in a healthy state:
* Overall status: The overall cluster status is `OK` and there are no warning or error status messages displayed.
* Monitors:  All of the monitors are `in quorum` and have individual status of `OK`.
* OSDs: All OSDs are `UP` and `IN`.
* MGRs: All Ceph managers are in the `Active` state.
* Placement groups: All PGs are in the `active+clean` state.

If your `rookctl status` output has deviations from the general good health described above, there may be an issue that needs to be investigated further.

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

In this guide, we will be upgrading a live Rook cluster running `v0.5.0` to the next available version of `v0.5.1`.
Let's get started!

### Operator
The Rook operator is the management brains of the cluster, so it should be upgraded first before other components.
In the event that the new version requires a migration of metadata or config, the operator is the one that would understand how to perform that migration.

The operator is managed by a Kubernetes deployment, so in order to upgrade the version of the operator pod, we will need to edit the image version of the pod template in the deployment spec.  This can be done with the following command:
```bash
kubectl -n rook-system set image deployment/rook-operator rook-operator=rook/rook:v0.5.1
```
Once the command is executed, Kubernetes will begin the flow of the deployment updating the operator pod.

#### Operator Health Verification
To verify the operator pod is `Running` and using the new version of `rook/rook:v0.5.1`, use the following commands:
```bash
OPERATOR_POD_NAME=$(kubectl -n rook-system get pods -l app=rook-operator -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook-system get pod ${OPERATOR_POD_NAME} -o jsonpath='{.status.phase}{"\n"}{.spec.containers[0].image}{"\n"}'
```

Once you've verified the operator is `Running` and on the new version, verify the health of the cluster is still OK.
Instructions for verifying cluster health can be found in the [health verification section](#health-verification).

### Agents
The Rook agents are deployed by the operator to run on every node. They are in charge of handling all operations related to the consumption of storage from the cluster.

The agents are deployed and managed by a Kubernetes daemonset. So in order to upgrade the version of the agent pods, we will need to edit the image version of the pod template in the daemonset spec and then delete each agent pod so that a new pod is created by the daemonset.

The following command updates the image of the agent daemonset:
```bash
kubectl -n rook-system set image daemonset/rook-agent rook-agent=rook/rook:v0.5.1
```

Once the daemonset is updated, we can begin deleting each agent pod **one at a time** and verifying a new one comes up to replace it that is running the new version.

The following is an example of deleting just one of the agent pods (note that the names of your agent pods will be different):
```bash
kubectl -n rook-system delete pod rook-agent-56m61
```

After all the agent pods have been updated, verify that they are `Running`.
```bash
kubectl -n rook-system get pod -l app=rook-agent
```

### API
Similar to the operator, the Rook API pod is managed by a Kubernetes deployment.
However, this time we will edit the deployment directly, because there is another field besides just the image version that needs to be updated in the Rook API deployment.
Begin editing the deployment with:
```bash
kubectl -n rook edit deployment rook-api
```

Note there are 2 fields that need to be updated.
These are the two relevant snippets you should update in your editor to use the new version of `rook/rook:v0.5.1` as shown:
```
        - name: ROOK_VERSION_TAG
          value: v0.5.1
```
```
        image: rook/rook:v0.5.1
        imagePullPolicy: IfNotPresent
        name: rook-api
```

After updating the version fields, the `rook-api` deployment should terminate the old pod and start a new one using the new version.
We can verify the new pod is in the `Running` state and using the new version with these commands:
```bash
API_POD_NAME=$(kubectl -n rook get pod -l app=rook-api -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook get pod ${API_POD_NAME} -o jsonpath='{.status.phase}{"\n"}{.spec.containers[0].image}{"\n"}'
```

Remember to verify the cluster health using the instructions found in the [health verification section](#health-verification).

### Monitors
There are multiple monitor pods to upgrade and they are each individually managed by their own replica set.
**For each** monitor's replica set, you will need to update the pod template spec's image version field to `rook/rook:v0.5.1`.
For example, we can update the replica set for `mon0` with:
```bash
kubectl -n rook set image replicaset/rook-ceph-mon0 rook-ceph-mon=rook/rook:v0.5.1
```

Once the replica set has been updated, we need to manually terminate the old pod which will trigger the replica set to create a new pod using the new version.
```bash
MON0_POD_NAME=$(kubectl -n rook get pod -l mon=rook-ceph-mon0 -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook delete pod ${MON0_POD_NAME}
```

After the new monitor pod comes up, we can verify that it's in the `Running` state and on the new version:
```bash
MON0_POD_NAME=$(kubectl -n rook get pod -l mon=rook-ceph-mon0 -o jsonpath='{.items[0].metadata.name}')
kubectl -n rook get pod ${MON0_POD_NAME} -o jsonpath='{.status.phase}{"\n"}{.spec.containers[0].image}{"\n"}'
```

At this point, it's very important to ensure that all monitors are `OK` and `in quorum`.
Refer to the [status output section](#status-output) for instructions.
If all of the monitors (and the cluster health overall) look good, then we can move on and repeat the same upgrade steps for the next monitor until all are completed.

**NOTE:** In the `v0.5.x` releases, the Rook operator is aggressive about replacing monitor pods that it finds out of quorum, even if it's only for a short time.
It is possible while upgrading your monitor pods that the operator will find them out of quorum and immediately replace them with a new monitor, such as `mon0` getting replaced by `mon3`.
This is okay as long as the cluster health looks good and all monitors eventually reach quorum again.

### Object Storage Daemons (OSDs)
The OSD pods can be managed in two different ways, depending on how you specified your storage configuration in your [Cluster spec](./cluster-crd.md#cluster-settings).  
* **Use all nodes:** all storage nodes in the cluster will be managed by a single daemon set.
Only the one daemon set will need to be edited to update the image version, then each OSD pod will need to be deleted so that a new pod will be created by the daemon set to take its place.
* **Specify individual nodes:** each storage node specified in the cluster spec will be managed by its own individual replica set.
Each of these replica sets will need to be edited to update the image version, then each OSD pod will need to be deleted so its replica set will start a new pod on the new version to replace it.

In this example, we are going to walk through the case where `useAllNodes: true` was set in the cluster spec, so there will be a single daemon set managing all the OSD pods.

Let's edit the image version of the pod template in the daemon set:
```bash
kubectl -n rook set image daemonset/rook-ceph-osd rook-ceph-osd=rook/rook:v0.5.1
```

Once the daemon set is updated, we can begin deleting each OSD pod **one at a time** and verifying a new one comes up to replace it that is running the new version.
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
Similar to the Rook API and operator, the Ceph manager pods are managed by a deployment.
We will edit the deployment to use the new image version of `rook/rook:v0.5.1`:
```bash
kubectl -n rook set image deployment/rook-ceph-mgr0 rook-ceph-mgr0=rook/rook:v0.5.1
```

To verify that the manager pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-mgr -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

### Optional Components
If you have optionally installed either [object storage](./object.md) or a [shared file system](./filesystem.md) in your Rook cluster, the sections below will provide guidance on how to update them as well.
They are both managed by deployments, which we have already covered in this guide, so the instructions will be brief.

#### Object Storage (RGW)
If you have object storage installed, first edit the RGW deployment to use the new image version of `rook/rook:v0.5.1`:
```bash
kubectl -n rook set image deployment/rook-ceph-rgw-default rook-ceph-rgw-default=rook/rook:v0.5.1
```

To verify that the RGW pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-rgw -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

#### Shared File System (MDS)
If you have a shared file system installed, first edit the MDS deployment to use the new image version of `rook/rook:v0.5.1`:
```bash
kubectl -n rook set image deployment/rook-ceph-mds rook-ceph-mds=rook/rook:v0.5.1
```

To verify that the MDS pod is `Running` and on the new version, use the following:
```bash
kubectl -n rook get pod -l app=rook-ceph-mds -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.phase}{" "}{.spec.containers[0].image}{"\n"}{end}'
```

## Completion
At this point, your Rook cluster should be fully upgraded to running version `rook/rook:v0.5.1` and the cluster should be healthy according to the steps in the [health verification section](#health-verification).

## Upgrading Kubernetes
:warning: Rook cluster installations on Kubernetes prior to version 1.7.x, use [ThirdPartyResource](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-third-party-resource/) that have been deprecated as of 1.7 and removed in 1.8. If upgrading your Kubernetes cluster Rook TPRs have to be migrated to CustomResourceDefinition (CRD) following [Kubernetes documentation](https://kubernetes.io/docs/tasks/access-kubernetes-api/migrate-third-party-resource/). Rook TPRs that require migration during upgrade are: 
- Cluster
- Pool
- ObjectStore
- Filesystem
- VolumeAttachment
