# Handling node drains through managed PodDisruptionBudgets

## Goals

- Handle and block node drains that would cause data unavailability and loss.
- Unblock drains dynamically so that a rolling upgrade is made possible.
- Allow for rolling upgrade of nodes in automated kubernetes environments like [cluster-api](https://github.com/kubernetes-sigs/cluster-api)

## Design

### OSDs

OSDs do not fit a single PodDisruptionBudget pattern. Ceph's ability to tolerate pod disruptions in one failure domain is dependent on the overall health of the cluster.
Even if an upgrade agent were only to drain one node at a time, Ceph would have to wait until there were no undersized PGs before moving on the next node.

The failure domain for the PDB is determined by the lowest failure domain
enforced by any Ceph RADOS pool.  For example, consider a cluster with two
pools, ``bigrbdpool`` that is associated with a CRUSH rule that enforces ``rack``
failure domain, and ``.mgr`` that is associated with a CRUSH rule that
enforces ``host`` failure domain.  In this case, the PDB failure domain will be ``host``.

#### Types of OSD PDBs:
- `Default` PDB:
    - Named `rook-ceph-osd`
    - Allows one healthy OSD to go down by setting `maxUnavailable=1` on any failure domain.
    - Sometimes one or more OSDs might down (non-overlapping drive failures, for example) without any node drained, but PGs are still `active+clean`. In that case, `maxUnavailable` is set to `1+number of down OSDs`
- `Blocking` PDBs
    - Named `rook-ceph-osd-<failureDomainType>-<FailureDomainName>`. For example: `rook-ceph-osd-zone-zone-a`
    - These PDBs are created on an entire failure domain.
    - `maxUnavailable` is set to 0 to prevent any OSD pod from draining.

We begin with creating the default PodDisruptionBudget for all the OSDs. Once the user drains a node and an OSD goes down, we determine the failure domain for the draining OSD (using the OSD deployment labels). Then we create blocking PodDisruptionBudgets (`maxUnavailable=0`) for all other failure domains and delete the main PodDisruptionBudget. This blocks OSDs from going down in multiple failure domains simultaneously.

Once the drained OSDs are back and all the pgs are `active+clean`, that is, the cluster has healed, the default PodDisruptionBudget is added back and blocking ones are deleted.

#### Detecting Node Drains:
Detecting drains is not easy as they are client-side operation. The client cordons a node and continuously attempts to evict all pods from the node until it succeeds. If a node on which the OSD is suppose to run is `unscheduleable` then the operator considers that node to be draining.

#### Example scenario:

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

1. A default PDB `rook-ceph-osd`, with `maxUnavailable=1`, is created for all OSDs.
2. An admin drains `Node a` for maintenance
3. The Rook operator
   - Notices that an OSD has gone down, for example, `osd.0` on `Node a` in `Zone x`:
   - Creates blocking PDBs `rook-ceph-osd-zone-zone-y` and `rook-ceph-osd-zone-zone-z`
   - Deletes the default PDB that covers all OSDs
   - Now all remaining OSDs in `zone x` are allowed to be drained
4. When `Node-a` is back, all of its OSDs are running and all PGs are `active+clean`:
   - Restores the default PDB `rook-ceph-osd` (maxUnavailable=1)
   - Deletes the blocking PDBs `rook-ceph-osd-zone-zone-y` and `rook-ceph-osd-zone-zone-z`

An example of an operator that will attempt to do rolling upgrades of nodes is the Machine Config Operator in OpenShift. Based on the
[SIG cluster lifecycle](https://github.com/kubernetes/community/tree/master/sig-cluster-lifecycle), Kubernetes deployments based on the cluster-api are
a common. This will also work to mitigate manual drains from accidentally disrupting storage.


#### Preventing unnecessary data migration:
- If a node is down due to planned maintenance, we don't want the data of the drained OSDs to be migrated to other OSDs, since the OSDs will return and recovery soon.
- The Rook operator adds the `noout` flag to the failure domain of the drained node.
- This `noout` flag is removed after the `OSDMaintenanceTimeout` has elapsed. `OSDMaintenanceTimeout` defaults to 30 minutes but can be configured from the cephCluster CR.
- `noout` is not added if an OSD is down but there is no drained node.

#### OSDs down due to reasons other than node drain:
- OSDs may be down for other reasons, e.g. drive failure.
- If all PGs are `active+clean` despite `down` OSD, the Rook operator will update the `maxUnavailable` count to `1+number of down OSDs` in the main PDB.
- This will allow other healthy OSDs to be drained.

### Monitor, Manager, MDS, RGW, rbd-mirror

Since there is no strict failure domain requirement for each of these pods, and they are not logically grouped, a static PDB suffices.

A single PodDisruptionBudget is created and owned by the respective controllers, and updated only according to changes in the CRDs that change the number of pods.

For example: In a deployment with three Monitors we can have PDB with the same labelSelector as the deployment and `maxUnavailable=1`.
When the Monitor count is increased to 5 (as is prudent in production), we can replace it with a PDB that has `maxUnavailable` set to `2`.
