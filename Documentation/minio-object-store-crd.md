---
title: Minio Object Store CRD
weight: 42
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
  credentials:
    name: access-keys
    namespace: rook-minio
  storageAmount: "10G"
  clusterDomain:
```

## Cluster Settings

### Minio Specific Settings

The settings below are specific to Minio object stores:

* `credentials`: This accepts the `name` and `namespace` strings of an existing Secret to specify the access credentials for the object store.
* `storageAmount`: The size of the volume that will be mounted at the data directory.
* `clusterDomain`: The local cluster domain for this cluster. This should be set if an alternative cluster domain is in use.  If not set, then the default of cluster.local will be assumed.  This field is needed to workaround https://github.com/minio/minio/issues/6775, and is expected to be removed in the future.

### Storage Scope

Under the `scope` field, a `StorageScopeSpec` can be specified to influence the scope or boundaries of storage that the cluster will use for its underlying storage. These properties are currently supported:

* `nodeCount`: The number of Minio instances to create.  Some of these instances may be scheduled on the same nodes, but exactly this many instances will be created and included in the cluster.
