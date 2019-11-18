# Monitor Health

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

You will always want an odd number of mons. Fifty percent of mons will not be sufficient to maintain quorum. If you had two mons and one
of them went down, you would have 1/2 of quorum. Since that is not a super-majority, the cluster would have to wait until the second mon is up again.

The number of mons to create in a cluster depends on your tolerance for losing a node. If you have 1 mon zero nodes can be lost
to maintain quorum. With 3 mons one node can be lost, and with 5 mons two nodes can be lost. Because the Rook operator will automatically
start a new new monitor if one dies, you typically only need three mons. The more mons you have, the more overhead there will be to make
a change to the cluster, which could become a performance issue in a large cluster.

## Mitigating Monitor Failure
Whatever the reason that a mon may fail (power failure, software crash, software hang, etc), there are several layers of mitigation in place
to help recover the mon. It is always better to bring an existing mon back up than to failover to bring up a new mon.

The Rook operator creates a mon with a ReplicaSet to ensure that the mon pod will always be restarted if it fails. If a mon pod stops
for any reason, Kubernetes will automatically start the pod up again.

In order for a mon to support a pod/node restart, you will need to set the `dataDirHostPath` so the mon metadata will be persisted to disk
in this folder. This will allow the mon to start back up with its existing metadata and continue where it left off even if the pod had
to be re-created. Without this host path, the mon cannot start again.

## Failing over a Monitor
If the mitigating steps for mon failure don't bring a mon back up, the operator will make the decision to terminate the old monitor pod
and bring up a new monitor with a new identity. This is an operation that must be done while there is mon quorum.

The operator checks for mon health every 45 seconds. If a monitor is down, the operator will wait 5 minutes before failing over the bad mon.
These two intervals can be configured as parameters to the rook [operator pod](/cluster/examples/kubernetes/ceph/operator.yaml). If the intervals are too short, it could be unhealthy if the mons are failed over too aggressively. If the intervals are too long, the cluster could be at risk of losing quorum if a new monitor is not brought up before another mon fails.
```
- name: ROOK_MON_HEALTHCHECK_INTERVAL
    value: "45s"
- name: ROOK_MON_OUT_TIMEOUT
    value: "600s"
```

### Example Failover
Rook will create mons with pod names such as mon0, mon1, and mon2. Let's say mon1 had an issue and the pod failed.
```
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mon
NAME                             READY     STATUS    RESTARTS   AGE
rook-ceph-mon0-7976n             1/1       Running   0          9m
rook-ceph-mon1-m675r             1/1       Error     0          9m
rook-ceph-mon2-cjbgk             1/1       Running   0          8m
```

After a failover, you will see the unhealthy mon removed and a new mon added such as mon0, mon2, and mon3. A fully healthy mon quorum is now running again.
```
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mon
NAME                             READY     STATUS    RESTARTS   AGE
rook-ceph-mon0-7976n             1/1       Running   0          9m
rook-ceph-mon2-cjbgk             1/1       Running   0          8m
rook-ceph-mon3-zjfb1             1/1       Running   0          1m
```
