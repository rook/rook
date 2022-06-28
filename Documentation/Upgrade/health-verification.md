---
title: Health Verification
---

Rook and Ceph upgrades are designed to ensure data remains available even while
the upgrade is proceeding. Rook will perform the upgrades in a rolling fashion
such that application pods are are not disrupted. To ensure the upgrades are
seamless, it is important to begin the upgrades with Ceph in a fully healthy state.
Let's first review some ways that you can verify the health of your cluster.

If you run into any issues during the upgrade, see the troubleshooting documentation:

* [General K8s troubleshooting](../Troubleshooting/common-issues.md)
* [Ceph common issues](../Troubleshooting/ceph-common-issues.md)
* [CSI common issues](../Troubleshooting/ceph-csi-common-issues.md)

### **Pods all Running**

In a healthy Rook cluster, all pods in the Rook namespace should be in the
`Running` (or `Completed`) state and have few, if any, pod restarts.

```console
ROOK_CLUSTER_NAMESPACE=rook-ceph
kubectl -n $ROOK_CLUSTER_NAMESPACE get pods
```

### **Status Output**

The [Rook toolbox](../Troubleshooting/ceph-toolbox.md) contains the Ceph tools that can give you status details of the cluster with the
`ceph status` command. Let's look at an output sample and review some of the details:

```console
TOOLS_POD=$(kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[*].metadata.name}')
kubectl -n $ROOK_CLUSTER_NAMESPACE exec -it $TOOLS_POD -- ceph status
```

The output should look similar to the following:
```
  cluster:
    id:     a3f4d647-9538-4aff-9fd1-b845873c3fe9
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum b,c,a
    mgr: a(active)
    mds: myfs-1/1/1 up  {0=myfs-a=up:active}, 1 up:standby-replay
    osd: 6 osds: 6 up, 6 in
    rgw: 1 daemon active

  data:
    pools:   9 pools, 900 pgs
    objects: 67  objects, 11 KiB
    usage:   6.1 GiB used, 54 GiB / 60 GiB avail
    pgs:     900 active+clean

  io:
    client:   7.4 KiB/s rd, 681 B/s wr, 11 op/s rd, 4 op/s wr
    recovery: 164 B/s, 1 objects/s
```

In the output above, note the following indications that the cluster is in a healthy state:

* Cluster health: The overall cluster status is `HEALTH_OK` and there are no warning or error status
  messages displayed.
* Monitors (mon):  All of the monitors are included in the `quorum` list.
* Manager (mgr): The Ceph manager is in the `active` state.
* OSDs (osd): All OSDs are `up` and `in`.
* Placement groups (pgs): All PGs are in the `active+clean` state.
* (If applicable) Ceph filesystem metadata server (mds): all MDSes are `active` for all filesystems
* (If applicable) Ceph object store RADOS gateways (rgw): all daemons are `active`

If your `ceph status` output has deviations from the general good health described above, there may
be an issue that needs to be investigated further. There are other commands you may run for more
details on the health of the system, such as `ceph osd status`. See the
[Ceph troubleshooting docs](https://docs.ceph.com/docs/master/rados/troubleshooting/) for help.

### Upgrading an unhealthy cluster

Rook will prevent the upgrade of the Ceph daemons if the health is in a `HEALTH_ERR` state.
If you desired to proceed with the upgrade anyway, you will need to set either
`skipUpgradeChecks: true` or `continueUpgradeAfterChecksEvenIfNotHealthy: true` as described in the
[cluster CR settings](../CRDs/Cluster/ceph-cluster-crd.md#cluster-settings).

### **Container Versions**

The container version running in a specific pod in the Rook cluster can be verified in its pod spec
output. For example, for the monitor pod `mon-b` we can verify the container version it is running
with the below commands:

```console
POD_NAME=$(kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -o custom-columns=name:.metadata.name --no-headers | grep rook-ceph-mon-b)
kubectl -n $ROOK_CLUSTER_NAMESPACE get pod ${POD_NAME} -o jsonpath='{.spec.containers[0].image}'
```

The status and container versions for all Rook pods can be collected all at once with the following
commands:

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0]}{"\n"}{end}' && \
kubectl -n $ROOK_CLUSTER_NAMESPACE get pod -o jsonpath='{range .items[*]}{.metadata.name}{"\n\t"}{.status.phase}{"\t\t"}{.spec.containers[0].image}{"\t"}{.spec.initContainers[0].image}{"\n"}{end}'
```

The `rook-version` label exists on Ceph resources. For various resource controllers, a
summary of the resource controllers can be gained with the commands below. These will report the
requested, updated, and currently available replicas for various Rook-Ceph resources in addition to
the version of Rook for resources managed by Rook. Note that the operator
and toolbox deployments do not have a `rook-version` label set.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'

kubectl -n $ROOK_CLUSTER_NAMESPACE get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"  \tsucceeded: "}{.status.succeeded}{"      \trook-version="}{.metadata.labels.rook-version}{"\n"}{end}'
```

### **Rook Volume Health**

Any pod that is using a Rook volume should also remain healthy:

* The pod should be in the `Running` state with few, if any, restarts
* There should be no errors in its logs
* The pod should still be able to read and write to the attached Rook volume.
