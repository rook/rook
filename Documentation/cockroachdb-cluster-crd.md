---
title: CockroachDB Cluster CRD
weight: 6000
---

# CockroachDB Cluster CRD

CockroachDB database clusters can be created and configured using the `clusters.cockroachdb.rook.io` custom resource definition (CRD).
Please refer to the the [user guide walk-through](cockroachdb.md) for complete instructions.
This page will explain all the available configuration options on the CockroachDB CRD.

## Sample

```yaml
apiVersion: cockroachdb.rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook-cockroachdb
  namespace: rook-cockroachdb
spec:
  scope:
    nodeCount: 3
    volumeClaimTemplates:
      - spec:
          accessModes: [ "ReadWriteOnce" ]
          # Uncomment and specify your StorageClass, otherwise
          # the cluster admin defined default StorageClass will be used.
          #storageClassName: "my-storage-class"
          resources:
            requests:
              storage: "100Gi"
  network:
    ports:
    - name: http
      port: 8080
    - name: grpc
      port: 26257
  secure: false
  cachePercent: 25
  maxSQLMemoryPercent: 25
  # A key/value list of annotations
  annotations:
  #  key: value
```

## Cluster Settings

### CockroachDB Specific Settings

The settings below are specific to CockroachDB database clusters:

* `secure`: `true` to create a secure cluster installation using certificates and encryption. `false` to create an insecure installation (strongly discouraged for production usage).  Currently, only insecure is supported.
* `cachePercent`: The total size used for caches, expressed as a percentage of total physical memory.
* `maxSQLMemoryPercent`: The maximum memory capacity available to store temporary data for SQL clients, expressed as a percentage of total physical memory.
* `annotations`: Key value pair list of annotations to add.

### Storage Scope

Under the `scope` field, a `StorageScopeSpec` can be specified to influence the scope or boundaries of storage that the cluster will use for its underlying storage.
These properties are currently supported:

* `nodeCount`: The number of CockroachDB instances to create.  Some of these instances may be scheduled on the same nodes, but exactly this many instances will be created and included in the cluster.
* `volumeClaimTemplates`: A list of PersistentVolumeClaim templates which must contain only **one or no** PersistentVolumeClaim. If no PersistentVolumeClaim is given an `emptyDir` will be given, meaning the instance data will be lost when a Pod is restarted. For an example of how PersistentVolumeClaim template should look, please look at the above [sample](#sample).

### Network

Under the `network` field, a `NetworkSpec` can be specified that describes network related settings of the cluster.
The properties that are currently supported are:

* `ports`: The port numbers to expose the CockroachDB services on, as shown in the [sample](#sample) above.  The supported port names are:
  * `http`: The port to bind to for HTTP requests such as the UI as well as health and debug endpoints.
  * `grpc`: The main port, served by gRPC, serves Postgres-flavor SQL, internode traffic and the command line interface.
