# Handling Openshift Fencing through managed MachineDisruptionBudgets

## Goals

- Use the MachineDisruptionBudget to ensure that OCP 4.x style fencing does not cause data unavailability and loss.

## What is fencing in OCP 4.x?

Openshift uses `Machines` and `MachineSets` from the [cluster-api](https://github.com/kubernetes-sigs/cluster-api) to dynamically provisions nodes. Fencing is a remediation method that reboots/deletes `Machine` CRDs to solve problems with automatically provisioned nodes.

Once [MachineHealthCheck controller](https://github.com/openshift/machine-api-operator#machine-healthcheck-controller) detects that a node is `NotReady` (or some other configured condition), it will remove the associated `Machine` which will cause the node to be deleted. The `MachineSet` controller will then replace the `Machine` via the machine-api. The exception is on baremetal platforms where fencing will reboot the underlying `BareMetalHost` object instead of deleting the `Machine`.


## Why can't we use `PodDisruptionBudget`?

Fencing does not use the eviction api. It is for `Machine`s and not `Pod`s.

## Will we need to do large storage rebalancing after fencing?

Hopefully not. On cloud platforms, the OSDs can be rescheduled on new nodes along with their backing PVs, and on baremetal where the local PVs are tied to a node, fencing will simply reboot the node instead of destroying it.


# Problem Statement
We need to ensure that only one node can be fenced at a time and that Ceph is fully recovered (has PGs clean) before any fencing is initiated. The available pattern for limiting fencing is the MachineDisruptionBudget which allows us to specify maxUnavailable. However, this won’t be sufficient to ensure that Ceph has recovered before fencing is initiated as MachineHealthCheck does not check anything other than the node state.

Therefore, we will control how many nodes match the MDB by dynamically adding and removing labels as well as dynamically updating the MDB. By manipulating the MDB into a state where desiredHealthy > currentHealthy, we can disable fencing on the nodes the MDB points to.

# Design:

We will implement two controllers `machinedisruptionbudget-controller` and the `machine-controller` to be implemented through the controller pattern describere [here](https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg#hdr-Controller_Writing_Tips). Each controller watches a set of object kinds and reconciles one.

The bottom line is that fencing is blocked if the PG state in not active+clean, but fencing continues on `Machine`s without the label which indicates that OSD resources are running there.

## machinedisruptionbudget-controller
This controller watches ceph PGs and CephClusters. We will ensure the reconciler is enqueued every 60s. It ensures that each CephCluster has a MDB created, and the MDB's value of maxUnvailable reflects the health of the Ceph Cluster's PGs.
If all PGs are clean, maxUnavailable = 1.
else, maxUnavailable = 0.

We can share a ceph health cache with the other controller-runtime reconcilers that have to watch the PG "cleanliness".

The MDB will target `Machine`s selected by a label maintainted by the `machine-controller`. The label is `fencegroup.rook.io/<cluster-name>`. 

## machine-controller
This controller watches OSDs and `Machine`s. It ensures that each `Machine` with OSDs from a `CephCluster` have the label `fencegroup.rook.io/<cluster-name>`, and those that do not have running OSDs do not have label.

This will ensure that no `Machine` without running OSDs will be protected by the MDB.

## Assumptions:
- We assume that the controllers will be able to reconcile multiple times in < 5 minutes as we know that fencing will happen only after a configurable timeout. The default timeout is 5 minutes.
  This is important because the MDB must be reconciled based on an accurate ceph health state in that time.


## Example Flows:

**Node needs to be fenced, the OSDs on the node are down too**

 - Node has NotReady condition.
 - Some Ceph PGs are not active+clean.
 - machinedisruptionbudget-controller sets maxUnavailable to 0 on the MachineDisruptionBudget.
 - MachineHealthCheck sees NotReady and attempts to fence after 5 minutes, but can't due to MDB
 - machine-controller notices all OSDs on the affected node are down and removes the node from the MDB.
 - MDB no longer covers the affected node, and MachineHealthCheck fences it.

**Node needs to be fenced, but the OSDs on the node are up**

 - Node has NotReady condition.
 - Ceph PGs are all active+clean so maxUnavailable remains 1 on the MDB.
 - MachineHealthCheck fences the Node.
 - Ceph resources on the node go down.
 - Some Ceph PGs are now not active+clean.
 - machinedisruptionbudget-controller sets maxUnavailble to 0 on the MachineDisruptionBudget.
 - If another labeled node needs to be fenced, it will only happen after the Ceph PGs become active+clean again when the OSDs are rescheduled and backfilled.
