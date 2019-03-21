# Safe disruption of nodes with osds


External operators and agents that provision,fence, or upgrade nodes should be able to use the kubernetes api to know how not to disrupt storage in the course of their operation.

External agents should only bring down one failure domain at once.


## Assumptions:
- The failure domain is node/host(or higher)
- To preserve availability, it is enough to ensure that only one host-that-has osds is down at once. 
- It is fine for the operator to constantly ping the cluster for it’s status.

## Proposed changes to `rook-ceph`:

- The nodes with osds can be labeled with `ceph.rook.io/has-osds=true`
- The nodes with osds can be annotated with the list of clusters that have osds on it.
    - The key can be `ceph.rook.io/tenant-clusters`
- The cluster CRD status can be updated with `CephCluster.Status.ClusterHealth` and `CephCluster.Status.ClusterHealthLastUpdated`.

## Example agent behaviour:


- The agent should only carry out disruptive actions on a node with `ceph.rook.io/has-osds=true` **only if** these conditions are satisfied:
    - the ClusterHealth is HEALTH_OK
    - `ClusterHealthLastUpdated` < THRESHOLD
    - `ClusterHealthLastUpdated` is more recent than the timestamp-of-last-disruptive-action.

After a disrupted node is brought back up, the agent could check the next nodes that have the same `ceph.rook.io/tenant-clusters`

## Why not use PodDisruptionBudget?

PodDisruptionBudget cannot work on the node level because it is not aware of node disruption. Since there can be multiple osds per node, disruption is not tied to a number of OSD pods. Ex: It is ok to have 5 osds down on one node (as all data will be replicated on the same node/failure-domain), but not ok to have 2 down accross nodes.