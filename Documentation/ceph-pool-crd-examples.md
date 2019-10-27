---
title: Block Pool CRD
weight: 2700
indent: true
---

# Ceph Block Pool CRD

Rook allows creation and customization of storage pools through the custom resource definitions (CRDs). The following settings are available for pools.

## Samples

### Replicated

For optimal performance, while also adding redundancy, this sample will configure Ceph to make three full copies of the data on multiple nodes.

> **NOTE**: This sample requires *at least 1 OSD per node*, with each OSD located on *3 different nodes*.

Each OSD must be located on a different node, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `host` and the `replicated.size` is set to `3`.

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

### Erasure Coded

This sample will lower the overall storage capacity requirement, while also adding redundancy by using [erasure coding](#erasure-coding).

> **NOTE**: This sample requires *at least 3 bluestore OSDs*.

The OSDs can be located on a single Ceph node or spread across multiple nodes, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `osd` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

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
(see the [OSD configuration settings](ceph-cluster-crd.md#osd-configuration-settings). Filestore OSDs have
[limitations](http://docs.ceph.com/docs/master/rados/operations/erasure-code/#erasure-coding-with-overwrites) that are unsafe and lower performance.

## Pool Settings

### Metadata

* `name`: The name of the pool to create.
* `namespace`: The namespace of the Rook cluster where the pool is created.

### Spec

* `replicated`: Settings for a replicated pool. If specified, `erasureCoded` settings must not be specified.
  * `size`: The desired number of copies to make of the data in the pool.
* `erasureCoded`: Settings for an erasure-coded pool. If specified, `replicated` settings must not be specified. See below for more details on [erasure coding](#erasure-coding).
  * `dataChunks`: Number of chunks to divide the original object into
  * `codingChunks`: Number of coding chunks to generate
* `failureDomain`: The failure domain across which the data will be spread. This can be set to a value of either `osd` or `host`, with `host` being the default setting. A failure domain can also be set to a different type (e.g. `rack`), if it is added as a `location` in the [Storage Selection Settings](ceph-cluster-crd.md#storage-selection-settings).
    If a `replicated` pool of size `3` is configured and the `failureDomain` is set to `host`, all three copies of the replicated data will be placed on OSDs located on `3` different Ceph hosts. This case is guaranteed to tolerate a failure of two hosts without a loss of data. Similarly, a failure domain set to `osd`, can tolerate a loss of two OSD devices.

    If erasure coding is used, the data and coding chunks are spread across the configured failure domain.

    > **NOTE**: Neither Rook, nor Ceph, prevent the creation of a cluster where the replicated data (or Erasure Coded chunks) can be written safely. By design, Ceph will delay checking for suitable OSDs until a write request is made and this write can hang if there are not sufficient OSDs to satisfy the request.
* `deviceClass`: Sets up the CRUSH rule for the pool to distribute data only on the specified device class. If left empty or unspecified, the pool will use the cluster's default CRUSH root, which usually distributes data over all OSDs, regardless of their class.
* `crushRoot`: The root in the crush map to be used by the pool. If left empty or unspecified, the default root will be used. Creating a crush hierarchy for the OSDs currently requires the Rook toolbox to run the Ceph tools described [here](http://docs.ceph.com/docs/master/rados/operations/crush-map/#modifying-the-crush-map).

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

If you do not have a sufficient number of hosts or OSDs for unique placement the pool can be created, although a PUT to the pool will hang.

Rook currently only configures two levels in the CRUSH map. It is also possible to configure other levels such as `rack` with the [Ceph tools](http://docs.ceph.com/docs/master/rados/operations/crush-map/).
