---
title: Minio Object Store CRD
weight: 7000
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
    # You can have multiple PersistentVolumeClaims in the volumeClaimTemplates list.
    # Be aware though that all PersistentVolumeClaim Templates will be used for each intance (see nodeCount).
    volumeClaimTemplates:
    - metadata:
        name: rook-minio-data1
      spec:
        accessModes: [ "ReadWriteOnce" ]
        # Uncomment and specify your StorageClass, otherwise
        # the cluster admin defined default StorageClass will be used.
        #storageClassName: "your-cluster-storageclass"
        resources:
          requests:
            storage: "8Gi"
    #- metadata:
    #    name: rook-minio-data2
    #  spec:
    #    accessModes: [ "ReadWriteOnce" ]
    #    # Uncomment and specify your StorageClass, otherwise
    #    # the cluster admin defined default StorageClass will be used.
    #    #storageClassName: "my-storage-class"
    #    resources:
    #      requests:
    #        storage: "8Gi"
  placement:
    tolerations:
    nodeAffinity:
    podAffinity:
    podAnyAffinity:
  credentials:
    name: minio-my-store-access-keys
    namespace: rook-minio
  clusterDomain:
  # A key/value list of annotations
  annotations:
  #  key: value
```

## Cluster Settings

### Minio Specific Settings

The settings below are specific to Minio object stores:

* `scope`: See [Storage Scope](#storage-scope).
* `credentials`: This accepts the `name` and `namespace` strings of an existing Secret to specify the access credentials for the object store.
* `clusterDomain`: The local cluster domain for this cluster. This should be set if an alternative cluster domain is in use.  If not set, then the default of cluster.local will be assumed.  This field is needed to workaround https://github.com/minio/minio/issues/6775, and is expected to be removed in the future.
* `annotations`: Key value pair list of annotations to add.

### Storage Scope

Under the `scope` field, a `StorageScopeSpec` can be specified to influence the scope or boundaries of storage that the cluster will use for its underlying storage. These properties are currently supported:

* `nodeCount`: The number of Minio instances to create.  Some of these instances may be scheduled on the same nodes, but exactly this many instances will be created and included in the cluster.
* `volumeClaimTemplates`: A list of one or more PersistentVolumeClaim templates to use for each Minio repliace. For an example of how the list should look like, please look at the above [sample](#sample).
