---
title: CephBlockPool CRD
---

Rook enables creation and customization of Ceph block storage (RBD) pools through custom
resource definitions (CRDs). Settings and options are detailed below.

## Examples

### Replicated RBD Pool

For optimal performance, while also adding redundancy, this example configures
a Ceph RBD pool with three full copies of user data, each on three different nodes.

!!! note
    This sample requires at least one OSD on each of at least three nodes.

Each Placement Group (PG) belonging to the specified pool will be placed on three
different OSDs, each a different node, because the [`failureDomain`](ceph-block-pool-crd.md#spec)
is set to `host` and the `replicated.size` is set to `3`.  This ensures data availability
when any one node (or any number of OSDs located on one node) is down due to maintenance
or failure.

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

### Erasure Coded RBD Pool

This example specifies a pool with an EC 4+2 data protection scheme.  For a
given amount of raw storage, this pool yields double the usable space compared
to the replicated `size 3` example above.  Redundancy is provided by
specifying an [erasure coding](#erasure-coding) profile with `codingChunks: 2`,
akin to a conventional RAID6 volume but with much greater flexibility.  Data
remains available if any one OSD is down and is preserved if any two are lost.

The downside is that write performance will be reduced compared to
the above replicated pool.  This additionally manifests in longer recovery
time from component failures and thus increased risk of data unavailability or
loss when a server or OSD drive fails.

!!! note
    This example requires *at least* six OSDs and six hosts.

The OSDs are spread across multiple nodes
because the [`failureDomain`](ceph-block-pool-crd.md#spec) is set to `host`.
Setting the `failureDomain` to `osd` is **not** a good idea for production data.
The `erasureCoded` chunk settings require at least six OSDs of the specified
deviceClass: four `dataChunks` + two `codingChunks`).


```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ecpool
  namespace: rook-ceph
spec:
  failureDomain: host
  erasureCoded:
    dataChunks: 4
    codingChunks: 2
  deviceClass: hdd
```

Various combinations of `dataChunks` and `codingChunks` values are possible, with
concomitant tradeoffs.  Note that for valuable production data a `codingChunks`
value of 1 is *extremely* risky and is not advised.  Pools holding production data
should be created with a `codingChunks` value of at least 2.  Larger values of
`codingChunks` allow data to remain available across additional nodes being
down and survive the complete loss of additional OSDs or nodes, at the expense
of reduced usable capacity and write performance.

Clusters with valuable production data should comprise at least three nodes when using
replicated pools and at least four when using erasure coding.  Specifically, an
erasure-coded pool should specify `failureDomain: host` and the cluster should comprise at
least `dataChunks+1` hosts.  There are certain operational flexibility advantages to
provisioning at least `dataChunks + codingChunks +1` hosts, including being able
to full use the aggregate raw capacity when the failure domains are not
all equal in CRUSH weight.

A very small cluster that cannot provision at least six nodes, at least at first, may
choose to select an EC profile with `dataChunks` set to `3` or `2`, to accommodate
minima of five and four hosts, respectively, albeit with lower usable:raw capacity
ratios. A table showing the usable to raw space yields of various erasure coding profiles
can be viewed [here](https://docs.ceph.com/en/latest/rados/operations/erasure-code/#erasure-coded-pool-overhead).

Performance-sensitive applications typically will not use erasure coding due to
the performance overhead of distributing and gathering the multiple chunks (shards)
across the cluster.

### Mirroring

RADOS Block Device (RBD) mirroring is a process of asynchronous replication of
Ceph block device images (or entire pools) between two or more Ceph clusters.
Mirroring ensures point-in-time consistent data replication, including reads
and writes, volume (image) resizing, snapshots, clones, and flattening.
RBD mirroring is useful when planning for Disaster Recovery.
Mirroring is feasible for clusters that are geographically distributed
with high network latency that precludes a `stretch mode` cluster or
when client write performance is more important than a zero Recovery Point
Objective (RPO).

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

This secret can then be fetched as follows:

```console
kubectl get secret -n rook-ceph pool-peer-token-replicapool -o jsonpath='{.data.token}'|base64 -d
eyJmc2lkIjoiOTFlYWUwZGQtMDZiMS00ZDJjLTkxZjMtMTMxMWM5ZGYzODJiIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFEN1psOWZ3V1VGRHhBQWdmY0gyZi8xeUhYeGZDUTU5L1N0NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjEwLjEwMS4xOC4yMjM6MzMwMCx2MToxMC4xMDEuMTguMjIzOjY3ODldIn0=
```

The secret must be decoded. The result will be another base64 encoded blob that you will import in the destination cluster:

```console
external-cluster-console # rbd mirror pool peer bootstrap import <token file path>
```

See the official `rbd-mirror` documentation on [how to add a bootstrap peer](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#bootstrap-peers).

!!! note
    Disabling mirroring for the CephBlockPool requires disabling mirroring on all the
    CephBlockPoolRadosNamespaces present underneath.

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

As an administrator I would like to maintain four copies of data, two in each datacenter and one
in each rack. This can be achieved by the following:

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
* `namespace`: The K8s namespace of the Rook cluster where the pool is created.

### Spec

* `replicated`: Settings for a replicated pool. When specified, `erasureCoded` settings cannot be specified.
    * `size`: The desired number of copies of data to maintain.
    * `requireSafeReplicaSize`: Set to `false` if you *really* want to create a pool with `size 1`, which will lead to permanent data loss sooner or later. Make sure you are *ABSOLUTELY CERTAIN* that is what you want.
    * `replicasPerFailureDomain`: The number of replicas to place in a given failure domain. For instance, if the failure domain is `datacenter` (as in a stretched cluster),
then you will have two replicas per `datacenter` where each replica ends up on a different host. This gives you a total of four replicas and for this, the `size` must be set to 4. The default value is 1.
    * `subFailureDomain`: Name of the CRUSH bucket representing a sub-failure domain. In a stretched configuration this option represent the leaf CRUSH bucket type where replicas will be placed. Imagine the cluster is stretched across two datacenters, you can then have two copies per `datacenter` and each copy on a different CRUSH bucket. The default is `host`.
* `erasureCoded`: Settings for an erasure-coded pool. If specified, `replicated` settings cannot be specified. See below for more details on [erasure coding](#erasure-coding).
    * `dataChunks`: Number of chunks to divide the original object into
    * `codingChunks`: Number of coding chunks to generate
* `failureDomain`: The failure domain across which the data will be spread. This can be set to a value of either `osd` or `host`, with `host` being the default setting. A failure domain can also be set to a different type (e.g. `rack`), if the OSDs are created on nodes with the supported [topology labels](../Cluster/ceph-cluster-crd.md#osd-topology). If the `failureDomain` is changed on the pool, the operator will create a new CRUSH rule and update the pool.
    If a `replicated` pool of size `3` is configured and the `failureDomain` is set to `host`, each copy of data will be placed on OSDs located on three different Ceph hosts. This case is guaranteed to tolerate a failure of two hosts without a loss of data. Similarly, a failure domain set to `osd`, can tolerate a loss of two OSD devices.  Setting the `failureDomain` to `osd` for valuable production data is strongly not recommended.

    If erasure coding is used, data and coding chunks are spread across the configured failure domains.

    !!! caution
        Neither Rook nor Ceph prevents the creation of a cluster or pool where replicated data (or Erasure Coded chunks) cannot be written safely. By design, Ceph will delay checking for suitable OSDs until a write request is made and this write can hang if there are not sufficient OSDs to satisfy the request.
* `deviceClass`: Configure the CRUSH rule for this pool to distribute data only on OSDs of the specified device class. If left empty or unspecified, the pool will use the cluster's default CRUSH root, which usually distributes data over all OSDs, regardless of their class. If `deviceClass` is specified on any pool, ensure that it is added to *every* pool in the cluster, otherwise Ceph will warn about pools with overlapping roots. Additionally, the PG autoscaler and the Ceph Balancer may be confounded.  Be careful to examine the `.mgr` pool's CRUSH rule too.
* `crushRoot`: The root in the CRUSH topology to be used by the pool. If left empty or unspecified, the default root will be used. Creating a custom CRUSH root for OSDs currently requires the Rook toolbox to run the Ceph tools described [here](http://docs.ceph.com/docs/master/rados/operations/crush-map/#modifying-the-crush-map).
* `enableCrushUpdates`: Enables Rook to update the pool's CRUSH rule using Pool Spec. Can cause data remapping if the CRUSH rule is changed, Defaults to `false`.
* `enableRBDStats`: Enables collecting RBD per-volume IO statistics by enabling
dynamic OSD performance counters. Defaults to `false`. For more info see
the [Ceph documentation](https://docs.ceph.com/docs/master/mgr/prometheus/#rbd-io-statistics).
Note that this will be much more performant when the `object-map` and `fast-diff` RBD feature
flags are present on RBD volumes.

* `name`: The name of the Ceph pool is based on the `metadata.name` of the CephBlockPool CR. Some built-in Ceph pools
    require names that are incompatible with K8s resource names. These special pools can be configured
    by setting this `name` to override the name of the Ceph pool that is created instead of using the `metadata.name` for the pool.
    Overriding only the following pool names is supported: `.nfs`, `.mgr`, and `.rgw.root`. See the example
    [builtin mgr pool](https://github.com/rook/rook/blob/master/deploy/examples/pool-builtin-mgr.yaml).
* `application`: The type of application set on the pool. By default, Ceph pools for CephBlockPools will be `rbd`,
    CephObjectStore pools will be `rgw`, and CephFilesystem pools will be `cephfs`.

* `parameters`: Sets any [parameters](https://docs.ceph.com/docs/master/rados/operations/pools/#set-pool-values) listed to the given pool
    * `target_size_ratio:` gives a hint (%) to the Ceph PG autoscaler in terms of expected consumption of the total cluster capacity of a given pool, for more info see the [ceph documentation](https://docs.ceph.com/docs/master/rados/operations/placement-groups/#specifying-expected-pool-size)
    * `compression_mode`: Configures data compression at the OSD level. If left unspecified, no compression is performed. Values supported are [these](https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#inline-compression):  `none`, `passive`, `aggressive`, and `force`.  In most cases `aggressive` is appropriate.  Specify `force` only if you really know what you're doing.

* `mirroring`: Configures Ceph `rbd-mirror` replicaton of the pool to a different Ceph cluster.
    * `enabled`: whether mirroring is enabled on that pool (default: false)
    * `mode`: mirroring mode to run, possible values are "pool", "image" or "init-only" (required). Refer to the [mirroring modes Ceph documentation](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/#enable-mirroring) for more details.
    * `snapshotSchedules`: schedule(s) snapshot at the **pool** level. One or more schedules are supported.
        * `interval`: frequency of the snapshots. The interval can be specified in days, hours, or minutes using d, h, m suffix respectively.
        * `startTime`: optional, determines at what time the snapshot process starts, specified using the ISO 8601 time format.
    * `peers`: to configure mirroring peers. See the prerequisite [RBD Mirror documentation](ceph-rbd-mirror-crd.md) first.
        * `secretNames`:  a list of peers to connect to. Currently **only a single** peer is supported where a peer represents a Ceph cluster.

* `statusCheck`: Configures pool mirroring status checks
    * `mirror`: displays the mirroring status
        * `disabled`: whether to enable or disable pool mirroring status
        * `interval`: time interval to refresh the mirroring status (default 60s)

* `quotas`: Set byte and object quotas. See the [ceph documentation](https://docs.ceph.com/en/latest/rados/operations/pools/#set-pool-quotas) for more info.
    * `maxSize`: quota in bytes as a string with quantity suffixes (e.g. "10Gi")
    * `maxObjects`: quota in objects as an integer

    !!! note
        A value of 0 disables the quota.

### Add specific pool properties

With `parameters` you can set any pool property:

```yaml
spec:
  parameters:
    <name of the parameter>: <parameter value>
```

For instance:

```yaml
spec:
  parameters:
    min_size: "1"
```

### Erasure Coding

[Erasure coding](http://docs.ceph.com/docs/master/rados/operations/erasure-code/) allows you to keep your data safe while reducing the storage overhead. Instead of creating multiple full replicas of the data,
erasure coding (EC) divides the original data into chunks of equal size, then generates additional chunks of that size for redundancy.  This is akin to RAID5 or RAID6 but much more flexible.

For example, if you have an RGW object of size 4MB, an erasure coding
profile with four data chunks would divide the object into two chunks of size
1 MB each (data chunks). Two coding (parity) chunks of size 1 MB will also be
computed. In total, 6MB will be stored in the cluster. The RGW object will be
able to suffer the loss of any two chunks and still be able to
reconstruct the original data, and if one chunk is lost the data will
remain available to clients.  When a number of chunks equal to the M (coding)
number are lost, in order to ensure strong consistency Ceph
will make that data *unavailable* until either at least one chunk becomes
available again or an administrator takes specific, manual action.  This
latter action is discouraged as it greatly increases the risk of permanent
data corruption or loss.

The number of data and coding chunks you choose will depend on your requirements for availability and data loss protection and how much storage overhead is acceptable.
Here are some examples to illustrate how the number of chunks affects the storage and loss toleration.  As we discuss above, M=1 is not recommended for irreplaceable data.
In most cases a value of K greater than 6 or 8 results in linear impact to write and recovery performance and demands additional failure domains, while the incremental
increase of the usable to raw capacities exhibits diminishing returns.

| Data chunks (K) | Coding chunks (M) | Total storage | Losses Tolerated | OSDs required |
| --------------- | ----------------- | ------------- | ---------------- | ------------- |
| 2               | 2                 | 2x            | 2                | 4             |
| 4               | 2                 | 1.5x          | 2                | 6             |
| 16              | 4                 | 1.25x         | 4                | 20            |

The `failureDomain` must be also be taken into account when determining the number of chunks. The failure domain determines the level in the Ceph CRUSH hierarchy where the chunks must be uniquely distributed. This decision will impact how many -- if any -- node losses or disk losses are tolerated.

* `host`: All chunks will be placed on unique hosts
* `osd`: All chunks will be placed on unique OSDs, which is not recommended for irreplaceable production data.

If you do not have a sufficient number of hosts or OSDs for unique placement the pool can be created, writing to the pool will hang and Ceph will report `undersized` or `incomplete` placement groups.

Rook currently only configures two levels in the CRUSH map. It is also possible to configure other levels such as `rack` with by adding [topology labels](../Cluster/ceph-cluster-crd.md#osd-topology) to the nodes.
