---
title: Block Pool CRD
weight: 2700
indent: true
---

# Ceph Block Pool CRD

Rook allows creation and customization of storage pools through the custom resource definitions (CRDs). The following settings are available
for pools.

## Samples

### Replicated

**NOTE** This example requires you to have **at least 3 OSDs each on a different node**.
This is because the `replicated.size: 3` will require at least 3 OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) to `host` (default), each OSD needs to be on a different nodes.

For optimal performance while adding redundancy to the system, configure the data to be copied in full to multiple locations.
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
```

### Erasure Coded

**NOTE** This example requires you to have **at least 3 bluestore OSDs on one or more nodes**.
This is because the below `erasureCoded` chunk settings require at least 3 bluestore OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) is set to `osd`, the OSDs can be on one or more nodes to be on a different nodes.

To lower your storage capacity requirements while adding redundancy, use [erasure coding](#erasure-coding).
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
```

High performance applications typically will not use erasure coding due to the performance overhead of creating and distributing the chunks in the cluster.

When creating an erasure-coded pool, it is highly recommended to create the pool when you have **bluestore OSDs** in your cluster
(see the [OSD configuration settings](ceph-cluster-crd.md#osd-configuration-settings). Filestore OSDs have
[limitations](http://docs.ceph.com/docs/luminous/rados/operations/erasure-code/#erasure-coding-with-overwrites) that are unsafe and lower performance.

## Pool Settings

### Metadata

- `name`: The name of the pool to create.
- `namespace`: The namespace of the Rook cluster where the pool is created.

### Spec

- `replicated`: Settings for a replicated pool. If specified, `erasureCoded` settings must not be specified.
  - `size`: The number of copies of the data in the pool.
- `erasureCoded`: Settings for an erasure-coded pool. If specified, `replicated` settings must not be specified. See below for more details on [erasure coding](#erasure-coding).
  - `dataChunks`: Number of chunks to divide the original object into
  - `codingChunks`: Number of redundant chunks to store
- `failureDomain`: The failure domain across which the replicas or chunks of data will be spread. Possible values per default are `osd` or `host`,
with the default of `host`. For example, if you have replication of size `3` and the failure domain is `host`, all three copies of the data will be
placed on osds that are found on unique hosts. In that case you would be guaranteed to tolerate the failure of two hosts. If the failure domain were `osd`,
you would be able to tolerate the loss of two devices. Similarly for erasure coding, the data and coding chunks would be spread across the requested failure domain.
<br>In case you added another location type to your nodes in the [Storage Selection Settings](ceph-cluster-crd.md#storage-selection-settings) (e.g. `rack`), you can also specify this type as your failure domain.
<br>**NOTE:** Neither Rook nor Ceph will prevent the user from creating a cluster where data (or chunks) cannot be replicated safely;
it is Ceph's design to delay checking for OSDs until a write request is made, and the write will hang if there are not sufficient OSDs to satisfy the request.
- `crushRoot`: The root in the crush map to be used by the pool. If left empty or unspecified, the default root will be used. Creating a crush hierarchy for the OSDs currently requires the Rook toolbox to run the Ceph tools described [here](http://docs.ceph.com/docs/master/rados/operations/crush-map/#modifying-the-crush-map).

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
- `host`: All chunks will be placed on unique hosts
- `osd`: All chunks will be placed on unique OSDs

If you do not have a sufficient number of hosts or OSDs for unique placement the pool can be created, although a PUT to the pool will hang.

Rook currently only configures two levels in the CRUSH map. It is also possible to configure other levels such as `rack` with the [Ceph tools](http://docs.ceph.com/docs/master/rados/operations/crush-map/).
