---
title: external Mon arbiter
target-version: release-1.17
---

# External Mon arbiter

The document describes a solution for maintaining Ceph Mon quorum for 2 Availability-Zone (AZ) setup to tolerate zone outage.

## Summary

To maintain quorum Ceph needs majority Mons to be available, otherwise access to all cluster will be lost.
For 2-AZ setup (aka 2 data-center) it becomes challenging because:

- if Mons are distributed equally across zones (2 Mons in each zone), then quorum will be lost in case of any zone outage 
- if Mons are distributed unequally (2+3), then quorum will be lost in case of major zone outage

Rook [stretched cluster](./ceph-stretch-cluster.md) already solved this problem by introducing a third "arbiter" zone hosting "arbiter" monitor.
Arbiter zone is hosted on one of K8s control-plane nodes because K8s cluster also requires 3AZ to tolerate zone outage.
However, there are environments where it is not possible to deploy workload to control-plane nodes:

- Managed K8s like GKE.
- Open-source managed K8s solutions like [Gardener](https://github.com/gardener/gardener).

In such cases, arbiter monitor should be deployed separately outside of K8s cluster and should be managed externally.
Currently, Rook supports only [external-cluster](./ceph-external-cluster.md) mode where all Mons are outside of the K8s cluster or normal mode where
all Mons are inside K8s cluster and if external Mon will join such cluster, then Rook will remove it from quorum automatically.

So the obvious solution would be to make Rook aware of external Mons by specifying external Mon IDs in `CephCluster` CR.

## Proposal details

Here is an example of how 2-AZ (also 2 node for simplicity) Ceph cluster setup with external Mon might look like:

1. Create 2-AZ Ceph cluster with one Mon per zone:
    ```diff yaml
    apiVersion: ceph.rook.io/v1
    kind: CephCluster
    spec:
      ...
      mon:
        # Spawn 2 Mons - one Mon in each AZ managed by Rook
        count: 2
        allowMultiplePerNode: false
    +   externalMons:
    +     - "mon-ext" # ID of external Mon.
    ```
2. When cluster is up, create Keyring, export `fsid`, `mon-initial-members`, and other required parameters to start external Mon daemon somewhere outside of K8s cluster and join it to existing quorum:
    ```yaml
    - args:
    - --fsid=<cluster fsid>
    - --mon-initial-members=<Mons managed by Rook>
    - --id=mon-ext
    ...
    ```
3. Move external Mon to [disallow-mode](https://docs.ceph.com/en/reef/rados/operations/change-mon-elections/#rados-operations-disallow-mode) to make sure that it won't be elected as a leader. The purpose of external mode is maintaining quorum to elect a new leader in case of zone outage. External Mon will likely have higher latency to the cluster so it is not desired to be elected.
    ```shell
    ceph mon add disallowed_leader mon-ext
    ```
4. Rook should be able to see `ext-mon` in Mon map and should not remove it.
