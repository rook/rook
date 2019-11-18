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

### Minio accessKey and secretKey

It is recommended to update the values of `accessKey` and `secretKey` in the `object-store.yaml` to a secure key pair, which is described in the [Minio client quickstart guide](https://docs.minio.io/docs/minio-client-quickstart-guide)

The default kubernetes secret resource will look like:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: access-keys
  namespace: rook-minio
type: Opaque
data:
  # Base64 encoded string: "TEMP_DEMO_ACCESS_KEY"
  username: VEVNUF9ERU1PX0FDQ0VTU19LRVk=
  # Base64 encoded string: "TEMP_DEMO_SECRET_KEY"
  password: VEVNUF9ERU1PX1NFQ1JFVF9LRVk=
```

You can use any mechanism to generate the new secure key pair, but you need to be sure the values are base64 encoded when being entered into kubernetes.
It is recommended to do the following in order to prevent new line feeds and carriage returns from being added into the base64 encoded value:

```console
$ cat minio-object-store.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: access-keys
  namespace: rook-minio
type: Opaque
data:
  username: #1
  password: #2


$ MINIO_ACCESS_KEY=$(echo -n "minio" | base64 -w0)
$ MINIO_SECRET_KEY=$(echo -n "minio123" | base64 -w0)
$ sed -i "s/#1/$MINIO_ACCESS_KEY/g" minio-object-store.yaml
$ sed -i "s/#2/$MINIO_SECRET_KEY/g" minio-object-store.yaml

$ cat minio-object-store.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: access-keys
  namespace: rook-minio
type: Opaque
data:
  username: bWluaW8K
  password: bWluaW8xMjMK
```

For further information in regards to this, please refer to the following related GitHub issues: [minio/minio](https://github.com/minio/minio/issues/7750) and [rook/minio](https://github.com/rook/rook/issues/3478)

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
