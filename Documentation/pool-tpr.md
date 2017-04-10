# Creating Rook Storage Pools
Rook allows creation and customization of storage pools through the third party resources (TPRs). The following settings are available
for pools.

## Sample
```
apiVersion: rook.io/v1alpha1
kind: Rookpool
metadata:
  name: ecpool
  namespace: rook
spec:
  replication:
  #  size: 3
  erasureCode:
    codingChunks: 2
    dataChunks: 2
```

## Pool Settings

### Metadata
- `name`: The name of the pool to create.
- `namespace`: The namespace of the Rook cluster where the pool is created.

### Spec
- `replication`: Settings for a replicated pool. If specified, `erasureCode` settings must not be specified.
  - `size`: The number of copies of the data in the pool.
- `erasureCode`: Settings for an erasure-coded pool. If specified, `replication` settings must not be specified.
  - `codingChunks`: Number of coding chunks per object in an erasure coded storage pool
  - `dataChunks`: Number of data chunks per object in an erasure coded storage pool