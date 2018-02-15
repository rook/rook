---
title: Pool
weight: 34
indent: true
---

# Pool CRD

Rook allows creation and customization of storage pools through the custom resource definitions (CRDs). The following settings are available
for pools.

## Sample

```yaml
apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: ecpool
  namespace: ceph
spec:
  replicated:
  #  size: 3
  erasureCoded:
    dataChunks: 2
    codingChunks: 1
  crushRoot: default
```

## Pool Settings

### Metadata

- `name`: The name of the pool to create.
- `namespace`: The namespace of the Ceph cluster where the pool is created.

### Spec

- `replicated`: Settings for a replicated pool. If specified, `erasureCoded` settings must not be specified.
  - `size`: The number of copies of the data in the pool.
- `erasureCoded`: Settings for an erasure-coded pool. If specified, `replicated` settings must not be specified.
  - `dataChunks`: Number of data chunks per object in an erasure coded storage pool
  - `codingChunks`: Number of coding chunks per object in an erasure coded storage pool
- `failureDomain`: The failure domain across which the replicas or chunks of data will be spread. Possible values are `osd` or `host`,
with the default of `host`.   For example, if you have replication of size `3` and the failure domain is `host`, all three copies of the data will be
placed on osds that are found on unique hosts. In that case you would be guaranteed to tolerate the failure of two hosts. If the failure domain were `osd`,
you would be able to tolerate the loss of two devices. Similarly for erasure coding, the data and coding chunks would be spread across the requested failure domain.
- `crushRoot`: The root in the crush map to be used by the pool. If left empty or unspecified, the default root will be used. Creating a crush hierarchy for the OSDs currently requires the Rook toolbox to run the Ceph tools described [here](http://docs.ceph.com/docs/master/rados/operations/crush-map/#modifying-the-crush-map).
