---
title: Monitor Health
---

Failure in a distributed system is to be expected. Ceph was designed from the ground up to deal with the failures of a distributed system.
At the next layer, Rook was designed from the ground up to automate recovery of Ceph components that traditionally required admin intervention.
Monitor health is the most critical piece of the equation that Rook actively monitors. If they are not in a good state,
the operator will take action to restore their health and keep your cluster protected from disaster.

The Ceph monitors (mons) are the brains of the distributed cluster. They control all of the metadata that is necessary
to store and retrieve your data as well as keep it safe. If the monitors are not in a healthy state you will risk losing all the data in your system.

## Monitor Identity

Each monitor in a Ceph cluster has a static identity. Every component in the cluster is aware of the identity, and that identity
must be immutable. The identity of a mon is its IP address.

To have an immutable IP address in Kubernetes, Rook creates a K8s service for each monitor. The clusterIP of the service will act as the stable identity.

When a monitor pod starts, it will bind to its podIP and it will expect communication to be via its service IP address.

## Monitor Quorum

Multiple mons work together to provide redundancy by each keeping a copy of the metadata. A variation of the distributed algorithm Paxos
is used to establish consensus about the state of the cluster. Paxos requires a super-majority of mons to be running in order to establish
quorum and perform operations in the cluster. If the majority of mons are not running, quorum is lost and nothing can be done in the cluster.

### How many mons?

Most commonly a cluster will have three mons. This would mean that one mon could go down and allow the cluster to remain healthy.
You would still have 2/3 mons running to give you consensus in the cluster for any operation.

For highest availability, an odd number of mons is required. Fifty percent of mons will not be sufficient to maintain quorum. If you had two mons and one
of them went down, you would have 1/2 of quorum. Since that is not a super-majority, the cluster would have to wait until the second mon is up again.
Rook allows an even number of mons for higher durability. See the [disaster recovery guide](../../Troubleshooting/disaster-recovery.md#restoring-mon-quorum) if quorum is lost and to recover mon quorum from a single mon.

The number of mons to create in a cluster depends on your tolerance for losing a node. If you have 1 mon zero nodes can be lost
to maintain quorum. With 3 mons one node can be lost, and with 5 mons two nodes can be lost. Because the Rook operator will automatically
start a new monitor if one dies, you typically only need three mons. The more mons you have, the more overhead there will be to make
a change to the cluster, which could become a performance issue in a large cluster.

## Mitigating Monitor Failure

Whatever the reason that a mon may fail (power failure, software crash, software hang, etc), there are several layers of mitigation in place
to help recover the mon. It is always better to bring an existing mon back up than to failover to bring up a new mon.

The Rook operator creates a mon with a Deployment to ensure that the mon pod will always be restarted if it fails. If a mon pod stops
for any reason, Kubernetes will automatically start the pod up again.

In order for a mon to support a pod/node restart, the mon metadata is persisted to disk, either under the `dataDirHostPath` specified
in the CephCluster CR, or in the volume defined by the `volumeClaimTemplate` in the CephCluster CR.
This will allow the mon to start back up with its existing metadata and continue where it left off even if the pod had
to be re-created. Without this persistence, the mon cannot restart.

## Failing over a Monitor

If a mon is unhealthy and the K8s pod restart or liveness probe are not sufficient to bring a mon back up, the operator will make the decision
to terminate the unhealthy monitor deployment and bring up a new monitor with a new identity.
This is an operation that must be done while mon quorum is maintained by other mons in the cluster.

The operator checks for mon health every 45 seconds. If a monitor is down, the operator will wait 10 minutes before failing over the unhealthy mon.
These two intervals can be configured as parameters to the CephCluster CR (see below). If the intervals are too short, it could be unhealthy if the mons are failed over too aggressively. If the intervals are too long, the cluster could be at risk of losing quorum if a new monitor is not brought up before another mon fails.

```yaml
healthCheck:
  daemonHealth:
    mon:
      disabled: false
      interval: 45s
      timeout: 10m
```

If you want to force a mon to failover for testing or other purposes, you can scale down the mon deployment to 0, then wait
for the timeout. Note that the operator may scale up the mon again automatically if the operator is restarted or if a full
reconcile is triggered, such as when the CephCluster CR is updated.

If the mon pod is in pending state and couldn't be assigned to a node (say, due to node drain), then the operator will wait for the timeout again before the mon failover. So the timeout waiting for the mon failover will be doubled in this case.

To disable monitor automatic failover, the `timeout` can be set to `0`, if the monitor goes out of quorum Rook will never fail it over onto another node.
This is especially useful for planned maintenance.

### Example Failover

Rook will create mons with pod names such as mon-a, mon-b, and mon-c. Let's say mon-b had an issue and the pod failed.

```console
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mon
NAME                               READY   STATUS    RESTARTS   AGE
rook-ceph-mon-a-74dc96545-ch5ns    1/1     Running   0          9m
rook-ceph-mon-b-6b9d895c4c-bcl2h   1/1     Error     2          9m
rook-ceph-mon-c-7d6df6d65c-5cjwl   1/1     Running   0          8m
```

After a failover, you will see the unhealthy mon removed and a new mon added such as mon-d. A fully healthy mon quorum is now running again.

```console
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mon
NAME                             READY     STATUS    RESTARTS   AGE
rook-ceph-mon-a-74dc96545-ch5ns    1/1     Running   0          19m
rook-ceph-mon-c-7d6df6d65c-5cjwl   1/1     Running   0          18m
rook-ceph-mon-d-9e7ea7e76d-4bhxm   1/1     Running   0          20s
```

From the toolbox we can verify the status of the health mon quorum:

```console
$ ceph -s
  cluster:
    id:     35179270-8a39-4e08-a352-a10c52bb04ff
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum a,b,d (age 2m)
    mgr: a(active, since 12m)
    osd: 3 osds: 3 up (since 10m), 3 in (since 10m)
[...]
```

## Automatic Monitor Failover

Rook will automatically fail over the mons when the following settings are updated in the
CephCluster CR:

- `spec.network.hostNetwork`: When enabled or disabled, Rook fails over all monitors, configuring them to enable or disable host networking.
- `spec.network.Provider` : When updated from being empty to "host", Rook fails over all monitors, configuring them to enable or disable host networking.
- `spec.network.multiClusterService`: When enabled or disabled, Rook fails over all monitors, configuring them to start (or stop) using service IPs compatible with the multi-cluster service.

## External Monitors

!!! attention
    This feature is experimental.

It is possible to have both Rook-managed and external monitors in the same Rook cluster.
One use case for this is 2 datacenter aka 2-AZ (Availability Zone) setup. For 2-AZ setup, Zone outage will lead to loss of the k8s control plane and half of the worker nodes hosting Rook mons.
In this case remaining half of the Rook cluster will not be able to form quorum and will be in a stuck state even if the other half of worker nodes are still up.
To avoid this situation, external mons can be used to form quorum and keep the cluster running.

If there are external monitors, Rook must be aware of them, otherwise Rook will remove the unknown mons from the quorum. This is done by setting the `mon.externalMonIDs` field in the CephCluster CR. The `mon.count` ignores the number of `mon.externalMonIDs`. For example, if `mon.count = 2`, Rook will create two internal mons no matter how many external mons are in the cluster and no matter what their health state might be. External monitors are supported only for the local Rook cluster running in normal mode. The external mons will be ignored for external clusters and stretch clusters.

Here is a step-by-step guide on how to add external monitors to a Rook cluster:

1. Create a CephCluster CR with the `mon.externalMonIDs` field set to the external monitor IDs. For example:

    ```yaml
    spec:
      mon:
        # Spawn 2 Mons - one Mon in each AZ managed by Rook
        count: 2
        allowMultiplePerNode: false
        # ID of external Mon
        externalMonIDs:
        - ext-mon-1 
    ```

    This will tell Rook to create two internal monitors in the cluster and to keep the external monitor with the ID `ext-mon-1` if it is found in the quorum.
    It is also possible to add `externalMonIDs` to an existing Cluster.
2. Wait until the Rook cluster and internal mons are up and running.
3. Manually deploy an external monitor with the ID `ext-mon-1` outside of the Rook cluster and its availability zones. See [ceph guide](https://docs.ceph.com/en/latest/rados/operations/add-or-rm-mons/#adding-removing-monitors) on deploying monitors.
4. Move the external Mon to [disallow-mode](https://docs.ceph.com/en/reef/rados/operations/change-mon-elections/#rados-operations-disallow-mode) to make sure that it won't be elected as a leader. The purpose of external mode is maintaining quorum to elect a new leader in case of a zone outage. The external Mon will likely have higher latency to the cluster so it should not be elected.
5. Check that the external monitor is in the quorum by running `ceph status` or `ceph quorum_status` from the toolbox.
6. Check that the external mon is added to the Rook mon endpoints:

    ```console
    $ kubectl -n rook-ceph get cm rook-ceph-mon-endpoints -o jsonpath='{.data.data}'
    a=10.100.68.61:6789,b=10.103.201.172:6789,ext-mon-1=10.102.136.102:6789
    ```
