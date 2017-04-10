# Creating Rook Storage Pools
Rook allows creation and customization of storage pools through the third party resources (TPRs). The following settings are available
for pools.

## Sample
```
apiVersion: rook.io/v1alpha1
kind: Rookpool
metadata:
  name: rook-ecpool
spec:
  name: ecpool
  namespace: rook
  replication:
  #  count: 3
  erasureCode:
    codingChunks: 2
    dataChunks: 2
```

## Pool Settings

### Metadata
- `name`: The name of the kubernetes resource. Must be unique across all Rook clusters. The naming convention is `clusterName-poolName`.
- `namespace`: The namespace where the Rook operator is running.

### Spec
- `name`: The name of the pool to create in the cluster.
- `namespace`: The namespace where the Rook cluster is running to create the pool.
- `replication`: Settings for a replicated pool. If specified, `erasureCode` settings must not be specified.
  - `count`: The number of copies of the data in the pool.
- `erasureCode`: Settings for an erasure-coded pool. If specified, `replication` settings must not be specified.
  - `codingChunks`: Number of coding chunks per object in an erasure coded storage pool
  - `dataChunks`: Number of data chunks per object in an erasure coded storage pool