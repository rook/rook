---
title: CephBlockPool CRD
---

Rook allows creation and customization of storage pools through the custom resource definitions (CRDs). The following settings are available for pools.

## Examples

### Replicated

For optimal performance, while also adding redundancy, this sample will configure Ceph to make three full copies of the data on multiple nodes.

!!! note
    This sample requires *at least 1 OSD per node*, with each OSD located on *3 different nodes*.

Each OSD must be located on a different node, because the [`failureDomain`](ceph-block-pool-crd.md#spec) is set to `host` and the `replicated.size` is set to `3`.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
  deviceClass: hdd
```

#### Hybrid Storage Pools

Hybrid storage is a combination of two different storage tiers. For example, SSD and HDD.
This helps to improve the read performance of cluster by placing, say, 1st copy of data on the higher performance tier (SSD or NVME) and remaining replicated copies on lower cost tier (HDDs).

**WARNING** Hybrid storage pools are likely to suffer from lower availability if a node goes down. The data across the two
tiers may actually end up on the same node, instead of being spread across unique nodes (or failure domains) as expected.
Instead of using hybrid pools, consider configuring [primary affinity](https://docs.ceph.com/en/latest/rados/operations/crush-map/#primary-affinity) from the toolbox.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
    hybridStorage:
      primaryDeviceClass: ssd
      secondaryDeviceClass: hdd
```

!!! important
    The device classes `primaryDeviceClass` and `secondaryDeviceClass` must have at least one OSD associated with them or else the pool creation will fail.

### Erasure Coded

This sample will lower the overall storage capacity requirement, while also adding redundancy by using [erasure coding](#erasure-coding).

!!! note
    This sample requires **at least 3 bluestore OSDs**.

The OSDs can be located on a single Ceph node or spread across multiple nodes, because the [`failureDomain`](ceph-block-pool-crd.md#spec) is set to `osd` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ecpool
  namespace: rook-ceph
spec:
  failureDomain: osd
  erasureCoded:
    dataChunks: 2
    codingChunks: 1
  deviceClass: hdd
```

High performance applications typically will not use erasure coding due to the performance overhead of creating and distributing the chunks in the cluster.

When creating an erasure-coded pool, it is highly recommended to create the pool when you have **bluestore OSDs** in your cluster
(see the [OSD configuration settings](../Cluster/ceph-cluster-crd.md#osd-configuration-settings). Filestore OSDs have
[limitations](http://docs.ceph.com/docs/master/rados/operations/erasure-code/#erasure-coding-with-overwrites) that are unsafe and lower performance.

### Mirroring

RADOS Block Device (RBD) mirroring is a process of asynchronous replication of Ceph block device images between two or more Ceph clusters.
Mirroring ensures point-in-time consistent replicas of all changes to an image, including reads and writes, block device resizing, snapshots, clones and flattening.
It is generally useful when planning for Disaster Recovery.
Mirroring is for clusters that are geographically distributed and stretching a single cluster is not possible due to high latencies.

The following will enable mirroring of the pool at the image level:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  replicated:
    size: 3
  mirroring:
    enabled: true
    mode: image
    # schedule(s) of snapshot
    snapshotSchedules:
      - interval: 24h # daily snapshots
        startTime: 14:00:00-05:00
```

Once mirroring is enabled, Rook will by default create its own [bootstrap peer token](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#bootstrap-peers) so that it can be used by another cluster.
The bootstrap peer token can be found in a Kubernetes Secret. The name of the Secret is present in the Status field of the CephBlockPool CR:

```yaml
status:
  info:
    rbdMirrorBootstrapPeerSecretName: pool-peer-token-replicapool
```

This secret can then be fetched like so:

```console
kubectl get secret -n rook-ceph pool-peer-token-replicapool -o jsonpath='{.data.token}'|base64 -d
eyJmc2lkIjoiOTFlYWUwZGQtMDZiMS00ZDJjLTkxZjMtMTMxMWM5ZGYzODJiIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFEN1psOWZ3V1VGRHhBQWdmY0gyZi8xeUhYeGZDUTU5L1N0NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjEwLjEwMS4xOC4yMjM6MzMwMCx2MToxMC4xMDEuMTguMjIzOjY3ODldIn0=
```

The secret must be decoded. The result will be another base64 encoded blob that you will import in the destination cluster:

```console
external-cluster-console # rbd mirror pool peer bootstrap import <token file path>
```

See the official rbd mirror documentation on [how to add a bootstrap peer](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#bootstrap-peers).

### Data spread across subdomains

Imagine the following topology with datacenters containing racks and then hosts:

```text
.
├── datacenter-1
│   ├── rack-1
│   │   ├── host-1
│   │   ├── host-2
│   └── rack-2
│       ├── host-3
│       ├── host-4
└── datacenter-2
    ├── rack-3
    │   ├── host-5
    │   ├── host-6
    └── rack-4
        ├── host-7
        └── host-8
```

As an administrator I would like to place 4 copies across both datacenter where each copy inside a datacenter is across a rack.
This can be achieved by the following:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  replicated:
    size: 4
    replicasPerFailureDomain: 2
    subFailureDomain: rack
```

## Pool Settings

### Metadata

* `name`: The name of the pool to create.
* `namespace`: The namespace of the Rook cluster where the pool is created.

### Spec

* `replicated`: Settings for a replicated pool. If specified, `erasureCoded` settings must not be specified.
    * `size`: The desired number of copies to make of the data in the pool.
    * `requireSafeReplicaSize`: set to false if you want to create a pool with size 1, setting pool size 1 could lead to data loss without recovery. Make sure you are *ABSOLUTELY CERTAIN* that is what you want.
    * `replicasPerFailureDomain`: Sets up the number of replicas to place in a given failure domain. For instance, if the failure domain is a datacenter (cluster is
stretched) then you will have 2 replicas per datacenter where each replica ends up on a different host. This gives you a total of 4 replicas and for this, the `size` must be set to 4. The default is 1.
    * `subFailureDomain`: Name of the CRUSH bucket representing a sub-failure domain. In a stretched configuration this option represent the "last" bucket where replicas will end up being written. Imagine the cluster is stretched across two datacenters, you can then have 2 copies per datacenter and each copy on a different CRUSH bucket. The default is "host".
* `erasureCoded`: Settings for an erasure-coded pool. If specified, `replicated` settings must not be specified. See below for more details on [erasure coding](#erasure-coding).
    * `dataChunks`: Number of chunks to divide the original object into
    * `codingChunks`: Number of coding chunks to generate
* `failureDomain`: The failure domain across which the data will be spread. This can be set to a value of either `osd` or `host`, with `host` being the default setting. A failure domain can also be set to a different type (e.g. `rack`), if the OSDs are created on nodes with the supported [topology labels](../Cluster/ceph-cluster-crd.md#osd-topology). If the `failureDomain` is changed on the pool, the operator will create a new CRUSH rule and update the pool.
    If a `replicated` pool of size `3` is configured and the `failureDomain` is set to `host`, all three copies of the replicated data will be placed on OSDs located on `3` different Ceph hosts. This case is guaranteed to tolerate a failure of two hosts without a loss of data. Similarly, a failure domain set to `osd`, can tolerate a loss of two OSD devices.

    If erasure coding is used, the data and coding chunks are spread across the configured failure domain.

    !!! caution
        Neither Rook, nor Ceph, prevent the creation of a cluster where the replicated data (or Erasure Coded chunks) can be written safely. By design, Ceph will delay checking for suitable OSDs until a write request is made and this write can hang if there are not sufficient OSDs to satisfy the request.
* `deviceClass`: Sets up the CRUSH rule for the pool to distribute data only on the specified device class. If left empty or unspecified, the pool will use the cluster's default CRUSH root, which usually distributes data over all OSDs, regardless of their class. If `deviceClass` is specified on any pool, ensure that it is added to every pool in the cluster, otherwise Ceph will warn about pools with overlapping roots.
* `crushRoot`: The root in the crush map to be used by the pool. If left empty or unspecified, the default root will be used. Creating a crush hierarchy for the OSDs currently requires the Rook toolbox to run the Ceph tools described [here](http://docs.ceph.com/docs/master/rados/operations/crush-map/#modifying-the-crush-map).
* `enableRBDStats`: Enables collecting RBD per-image IO statistics by enabling dynamic OSD performance counters. Defaults to false. For more info see the [ceph documentation](https://docs.ceph.com/docs/master/mgr/prometheus/#rbd-io-statistics).
* `name`: The name of Ceph pools is based on the `metadata.name` of the CephBlockPool CR. Some built-in Ceph pools
    require names that are incompatible with K8s resource names. These special pools can be configured
    by setting this `name` to override the name of the Ceph pool that is created instead of using the `metadata.name` for the pool.
    Only the following pool names are supported: `.nfs`, `.mgr`, and `.rgw.root`. See the example
    [builtin mgr pool](https://github.com/rook/rook/blob/master/deploy/examples/pool-builtin-mgr.yaml).
* `application`: The type of application set on the pool. By default, Ceph pools for CephBlockPools will be `rbd`,
    CephObjectStore pools will be `rgw`, and CephFilesystem pools will be `cephfs`.

* `parameters`: Sets any [parameters](https://docs.ceph.com/docs/master/rados/operations/pools/#set-pool-values) listed to the given pool
    * `target_size_ratio:` gives a hint (%) to Ceph in terms of expected consumption of the total cluster capacity of a given pool, for more info see the [ceph documentation](https://docs.ceph.com/docs/master/rados/operations/placement-groups/#specifying-expected-pool-size)
    * `compression_mode`: Sets up the pool for inline compression when using a Bluestore OSD. If left unspecified does not setup any compression mode for the pool. Values supported are the same as Bluestore inline compression [modes](https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#inline-compression), such as `none`, `passive`, `aggressive`, and `force`.

* `mirroring`: Sets up mirroring of the pool
    * `enabled`: whether mirroring is enabled on that pool (default: false)
    * `mode`: mirroring mode to run, possible values are "pool" or "image" (required). Refer to the [mirroring modes Ceph documentation](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#enable-mirroring) for more details.
    * `snapshotSchedules`: schedule(s) snapshot at the **pool** level. One or more schedules are supported.
        * `interval`: frequency of the snapshots. The interval can be specified in days, hours, or minutes using d, h, m suffix respectively.
        * `startTime`: optional, determines at what time the snapshot process starts, specified using the ISO 8601 time format.
    * `peers`: to configure mirroring peers. See the prerequisite [RBD Mirror documentation](ceph-rbd-mirror-crd.md) first.
        * `secretNames`:  a list of peers to connect to. Currently **only a single** peer is supported where a peer represents a Ceph cluster.

* `statusCheck`: Sets up pool mirroring status
    * `mirror`: displays the mirroring status
        * `disabled`: whether to enable or disable pool mirroring status
        * `interval`: time interval to refresh the mirroring status (default 60s)

* `quotas`: Set byte and object quotas. See the [ceph documentation](https://docs.ceph.com/en/latest/rados/operations/pools/#set-pool-quotas) for more info.
    * `maxSize`: quota in bytes as a string with quantity suffixes (e.g. "10Gi")
    * `maxObjects`: quota in objects as an integer

    !!! note
        A value of 0 disables the quota.

### Add specific pool properties

With `poolProperties` you can set any pool property:

```yaml
spec:
  parameters:
    <name of the parameter>: <parameter value>
```

For instance:

```yaml
spec:
  parameters:
    min_size: 1
```

### Erasure Coding

[Erasure coding](http://docs.ceph.com/docs/master/rados/operations/erasure-code/) allows you to keep your data safe while reducing the storage overhead. Instead of creating multiple replicas of the data,
erasure coding divides the original data into chunks of equal size, then generates extra chunks of that same size for redundancy.

For example, if you have an object of size 2MB, the simplest erasure coding with two data chunks would divide the object into two chunks of size 1MB each (data chunks). One more chunk (coding chunk) of size 1MB will be generated. In total, 3MB will be stored in the cluster. The object will be able to suffer the loss of any one of the chunks and still be able to reconstruct the original object.

The number of data and coding chunks you choose will depend on your resiliency to loss and how much storage overhead is acceptable in your storage cluster.
Here are some examples to illustrate how the number of chunks affects the storage and loss toleration.

| Data chunks (k) | Coding chunks (m) | Total storage | Losses Tolerated | OSDs required |
| --------------- | ----------------- | ------------- | ---------------- | ------------- |
| 2               | 1                 | 1.5x          | 1                | 3             |
| 2               | 2                 | 2x            | 2                | 4             |
| 4               | 2                 | 1.5x          | 2                | 6             |
| 16              | 4                 | 1.25x         | 4                | 20            |

The `failureDomain` must be also be taken into account when determining the number of chunks. The failure domain determines the level in the Ceph CRUSH hierarchy where the chunks must be uniquely distributed. This decision will impact whether node losses or disk losses are tolerated. There could also be performance differences of placing the data across nodes or osds.

* `host`: All chunks will be placed on unique hosts
* `osd`: All chunks will be placed on unique OSDs

If you do not have a sufficient number of hosts or OSDs for unique placement the pool can be created, writing to the pool will hang.

Rook currently only configures two levels in the CRUSH map. It is also possible to configure other levels such as `rack` with by adding [topology labels](../Cluster/ceph-cluster-crd.md#osd-topology) to the nodes.
