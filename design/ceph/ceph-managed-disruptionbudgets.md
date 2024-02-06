# Handling node drains through managed PodDisruptionBudgets.

## Goals

- Handle and block node drains that would cause data unavailability and loss.
- Unblock drains dynamically so that a rolling upgrade is made possible.
- Allow for rolling upgrade of nodes in automated kubernetes environments like [cluster-api](https://github.com/kubernetes-sigs/cluster-api)



## Design

### OSDs

OSDs do not fit under the single PodDisruptionBudget pattern. Ceph's ability to tolerate pod disruptions in one failure domain is dependent on the overall health of the cluster.
Even if an upgrade agent were only to drain one node at a time, Ceph would have to wait until there were no undersized PGs before moving on the next.

The failure domain will be determined by the smallest failure domain of all the Ceph Pools in that cluster.
We begin with creating a single PodDisruptionBudget for all the OSD with maxUnavailable=1. This will allow one OSD to go down anytime. Once the user drains a node and an OSD goes down, we determine the failure domain for the draining OSD (using the OSD deployment labels). Then we create blocking PodDisruptionBudgets (maxUnavailable=0) for all other failure domains and delete the main PodDisruptionBudget. This blocks OSDs from going down in multiple failure domains simultaneously.

Once the drained OSDs are back and all the pgs are active+clean, that is, the cluster is healed, the default PodDisruptionBudget (with maxUnavailable=1) is added back and the blocking ones are deleted. User can also add a timeout for the pgs to become healthy. If the timeout exceeds, the operator will ignore the pg health, add the main PodDisruptionBudget and delete the blocking ones.

Detecting drains is not easy as they are a client side operation. The client cordons the node and continuously attempts to evict all pods from the node until it succeeds. Whenever an OSD goes into pending state, that is, `ReadyReplicas` count is 0, we assume that some drain operation is happening.

Example scenario:

- Zone x
  - Node a
    - osd.0
    - osd.1
- Zone y
  - Node b
    - osd.2
    - osd.3
- Zone z
  - Node c
    - osd.4
    - osd.5

1. Rook Operator creates a single PDB that covers all OSDs with maxUnavailable=1.
2. When Rook Operator sees an OSD go down (for example, osd.0 goes down):
   - Create a PDB for each failure domain (zones y and z) with maxUnavailable=0 where the OSD did *not* go down.
   - Delete the original PDB that covers all OSDs
   - Now all remaining OSDs in zone x would be allowed to be drained
3. When Rook sees the OSDs are back up and all PGs are clean
   - Restore the PDB that covers all OSDs with maxUnavailable=1
   - Delete the PDBs (in zone y and z) where maxUnavailable=0

An example of an operator that will attempt to do rolling upgrades of nodes is the Machine Config Operator in openshift. Based on what I have seen in
[SIG cluster lifecycle](https://github.com/kubernetes/community/tree/master/sig-cluster-lifecycle), kubernetes deployments based on cluster-api approach will be
a common way of deploying kubernetes. This will also work to mitigate manual drains from accidentally disrupting storage.

When an node is drained, we will also delay it's DOWN/OUT process by placing a noout on that node. We will remove that noout after a timeout.

An OSD can be down due to reasons other than node drain, say, disk failure. In such a situation, if the pgs are unhealthy then rook will create a blocking PodDisruptionBudget on other failure domains to prevent further node drains on them. `noout` flag won't be set on node this is case. If the OSD is down but all the pgs are `active+clean`, the cluster will be treated as fully healthy. The default PodDisruptionBudget (with maxUnavailable=1) will be added back and the blocking ones will be deleted.

### Mon, Mgr, MDS, RGW, RBDMirror

Since there is no strict failure domain requirement for each of these, and they are not logically grouped, a static PDB will suffice.

A single PodDisruptionBudget is created and owned by the respective controllers, and updated only according to changes in the CRDs that change the amount of pods.

Eg: For a 3 Mon configuration, we can have PDB with the same labelSelector as the Deployment and have maxUnavailable as 1.
If the mon count is increased to 5, we can replace it with a PDB that has maxUnavailable set to 2.
