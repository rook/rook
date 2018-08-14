---
title: Minio Object Store CRD
weight: 38
indent: true
---

# Minio Object Store CRD
Minio object stores can be created and configured using the `objectstores.minio.rook.io` custom resource definition (CRD). Complete instructions can be found in the [Rook Minio Documentation](minio-object-store.md).

## Sample

```yaml
apiVersion: minio.rook.io/v1alpha1
kind: ObjectStore
metadata:
  name: my-store
  namespace: rook-minio
spec:
  scope:
    nodeCount: 4
  placement:
    tolerations:
    nodeAffinity:
    podAffinity:
    podAnyAffinity:
  port: 9000
  credentials:
    name: access-keys
    namespace: rook-minio
  storageAmount: "10G"
```

## Cluster Settings

### Minio Specific Settings

The settings below are specific to Minio object stores:

* `port`: The internal port exposed internal to the cluster by the Minio service.
* `credentials`: This accepts the `name` and `namespace` strings of an existing Secret to specify the access credentials for the object store.
* `storageAmount`: The size of the volume that will be mounted at the data directory.

### Storage Scope

Under the `scope` field, a `StorageScopeSpec` can be specified to influence the scope or boundaries of storage that the cluster will use for its underlying storage. These properties are currently supported:

* `nodeCount`: The number of Minio instances to create.  Some of these instances may be scheduled on the same nodes, but exactly this many instances will be created and included in the cluster.
