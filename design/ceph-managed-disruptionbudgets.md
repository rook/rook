# Handling node drains through managed PodDisruptionBudgets.

## Goals

- Handle and block node drains that would cause data unavailability and loss.
- Unblock drains dynamically so that a rolling upgrade is made possible.
- Allow for rolling upgrade of nodes in automated kubernetes environments like [cluster-api](https://github.com/kubernetes-sigs/cluster-api)



## Design

### OSDs

OSDs do not fit under the single PodDisruptionBudget pattern. Ceph's ability to tolerate pod disruptions in one failure domain is dependent on the overall health of the cluster.
Even if an upgrade agent were only to drain one node at a time, Ceph would have to wait until there were no undersized PGs before moving on the the next.
Therefore, we will create a PodDisruptionBudget per failure domain that does not allow any evictions by default.
When attempts to drain are detected, we will delete PodDisruption budget on one node at a time, progressing to the next only after ceph is healthy enough to avoid data loss/unavailability.
The failure domain will be determined by the smallest failure domain of all the Ceph Pools in that cluster.

Detecting drains is not easy as they are a client side operation. The client cordons the node and continuously attempts to evict all pods from the node until it succeeds.
We will use a heuristic to detect drains. We will create a canary deployment for each node with a nodeSelector for that node. Since it is likely that pod will only be
removed from the node in the event of a drain, we will rely on the assumption that if that pod is not running, that node is being drained. This will not be a dangerous assumption
as false positives for drains are not dangerous in this use case.

Example flow:
- A Ceph pool CRD is created.
- The Rook operator creates a PDB with maxUnvailable of 0 for each failure domain.
- A cluster upgrade agent wants to perform a kernel upgrade on the nodes.
- It attempts to drain 1 or more nodes.
- The drain attempt successfully evicts the canary pod.
- The Rook operator interprets this as a drain request that it can grant by deleting the PDB.
- The Rook operator deletes one PDB, and the blocked drain on that failure domain completes.
- The OSDs on that node comeback up and all the necessary backfilling occurs, and all the osds are active+clean.
- The Rook operator recreates the PDB on that failure domain.
- The process is repeated with the subsequent nodes/failure domains.

An example of an operator that will attempt to do rolling upgrades of nodes is the Machine Config Operator in openshift. Based on what I have seen in
[SIG cluster lifecycle](https://github.com/kubernetes/community/tree/master/sig-cluster-lifecycle), kubernetes deployments based on cluster-api approach will be
a common way of deploying kubernetes. This will also work to mitigate manual drains from accidentally disrupting storage.

When an node is drained, we will also delay it's DOWN/OUT process by placing a noout on that node. We will remove that noout after a timeout.

### Mon, Mgr, MDS, RGW, RBDMirror

Since there is no strict failure domain requirement for each of these, and they are not logically grouped, a stactic PDB will suffice.

A single PodDisruptionBudget is created and owned by the respective controllers, and updated only according to changes in the CRDs that change the amount of pods.

Eg: For a 3 Mon configuration, we can have PDB with the same labelSelector as the Deployment and have maxUnavailable as 1.
If the mon count is increased to 5, we can replace it with a PDB that has maxUnavailable set to 2.
