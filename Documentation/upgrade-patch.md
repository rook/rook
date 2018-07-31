---
title: Patch Upgrades
weight: 61
indent: true
---

# 0.8 Patch Upgrades

The changes in patch releases are scoped to the minimal changes necessary and are expected to be straight forward to upgrade.
This guide will walk you through the manual steps to upgrade the software in a Rook cluster from an 0.8 version to another patched version of 0.8.
For example, upgrade from `v0.8.0` to `v0.8.1`.

After each component is upgraded, it is important to verify that the cluster returns to a healthy and fully functional state.

## Considerations
With this manual upgrade guide, there are a few notes to consider:
* **WARNING:** Upgrading a Rook cluster is a manual process in its very early stages.  There may be unexpected issues or obstacles that damage the integrity and health of your storage cluster, including data loss.
* This guide assumes that your Rook operator and its agents are running in the `rook-ceph-system` namespace. It also assumes that your Rook cluster is in the `rook-ceph` namespace.  If any of these components is in a different namespace, search/replace all instances of `-n rook-ceph-system` and `-n rook-ceph` in this guide with `-n <your namespace>`.

## Prerequisites
In order to successfully upgrade a Rook cluster, the following prerequisites must be met:
* The cluster should be in a healthy state with full functionality.
Review the [health verification section](upgrade.md#health-verification) in the main upgrade guide in order to verify your cluster is in a good starting state.
* All pods consuming Rook storage should be created, running, and in a steady state.  No Rook persistent volumes should be in the act of being created or deleted.

## Upgrade Process
The general flow of the upgrade process will be to upgrade the version of a Rook pod, verify the pod is running with the new version, then verify that the overall cluster health is still in a good state.

In this guide, we will be upgrading a live Rook cluster running `v0.8.0` to version `v0.8.1`.

### Operator
The operator controls upgrading all the components and is generally the first component to be updated.
The operator can be updated by setting the deployment version.

```bash
kubectl -n rook-ceph-system set image deploy/rook-ceph-operator rook-ceph-operator=rook/ceph:v0.8.1
```

The operator pod will automatically be restarted by Kubernetes with the new version.

Once you've verified the operator is `Running` and on the new version, verify the health of the cluster is still OK.
Instructions for verifying cluster health can be found in the [health verification section](upgrade.md#health-verification).

### System Daemons
The pods in the rook-ceph-system namespace will all be updated automatically when the operator is updated. After the operator is updated, you will see the `rook-ceph-agent` and `rook-discover` pods restarted on the new version.

### Monitors
There are multiple monitor pods to upgrade and they are each individually managed by their own replica set.
**For each** monitor's replica set, you will need to update the pod template spec's image version field to `rook/ceph:v0.8.1`.
For example, we can update the replica set for `mon0` with:
```bash
kubectl -n rook-ceph set image replicaset/rook-ceph-mon0 rook-ceph-mon=rook/ceph:v0.8.1
```

Once the replica set has been updated, we need to manually terminate the old pod which will trigger the replica set to create a new pod using the new version.
```bash
kubectl -n rook-ceph delete pod -l mon=rook-ceph-mon0
```

At this point, it's very important to ensure that all monitors are `OK` and `in quorum`.
Refer to the [status output section](upgrade.md#status-output) for instructions.
If all of the monitors (and the cluster health overall) look good, then we can move on and repeat the same upgrade steps for the next monitor until all are completed.

### Object Storage Daemons (OSDs)
The automatic upgrade of OSD pods has been implemented by the operator. Within a minute of starting the operator on the new version, you should see the osd pods automatically started on the new version.
Going forward, if there is any change needed to the OSD deployment as determined by the operator, the OSD pods will automatically be updated and restarted.
For example, the OSDs will be automatically updated when:
- The version of the operator container changes
- The `resources` or `placement` elements are changed in the cluster CRD

One by one, as each of the OSDs are updated the operator will wait for the OSD pod to be running again before continuing with the next OSD.
In some scenarios, the operator will need to be restarted in order to apply the changes to the OSD deployment specs.

### Ceph Manager
To update the Ceph mgrs, edit the deployment image version:
```bash
kubectl -n rook-ceph set image deploy/rook-ceph-mgr-a rook-ceph-mgr-a=rook/ceph:v0.8.1
```

#### Object Storage (RGW)
If you have object storage installed, edit the RGW deployment to use the new image version:
```bash
kubectl -n rook-ceph set image deploy/rook-ceph-rgw-my-store rook-ceph-rgw-my-store=rook/ceph:v0.8.1
```

#### Shared File System (MDS)
If you have a shared file system installed, edit the MDS deployment to use the new image version:
```bash
kubectl -n rook-ceph set image deploy/rook-ceph-mds-myfs rook-ceph-mds-myfs=rook/ceph:v0.8.1
```

## Completion
At this point, your Rook cluster should be fully upgraded to running version `rook/ceph:v0.8.1` and the cluster should be healthy according to the steps in the [health verification section](upgrade.md#health-verification).
