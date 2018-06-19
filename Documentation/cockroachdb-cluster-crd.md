---
title: CockroachDB Cluster
weight: 37
indent: true
---

# CockroachDB Cluster CRD

CockroachDB database clusters can be created and configuring using the `clusters.cockroachdb.rook.io` custom resource definition (CRD).
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
  network:
    ports:
    - name: http
      port: 8080
    - name: grpc
      port: 26257
  secure: false
  volumeSize: 100Gi
  cachePercent: 25
  maxSQLMemoryPercent: 25
```

## Cluster Settings

### CockroachDB Specific Settings

The settings below are specific to CockroachDB database clusters:

* `secure`: `true` to create a secure cluster installation using certificates and encryption. `false` to create an insecure installation (strongly discouraged for production usage).  Currently, only insecure is supported.
* `volumeSize`: Each database instance will get an underlying persistent data volume created to store its instance data using the default storage class.  This value represents the size of the volume that will be created.  The value should be expressed in the [standard Kubernetes resource format](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#meaning-of-memory).
* `cachePercent`: The total size used for caches, expressed as a percentage of total physical memory.
* `maxSQLMemoryPercent`: The maximum memory capacity available to store temporary data for SQL clients, expressed as a percentage of total physical memory.

### Storage Scope

Under the `scope` field, a `StorageScopeSpec` can be specified to influence the scope or boundaries of storage that the cluster will use for its underlying storage.
These properties are currently supported:

* `nodeCount`: The number of CockroachDB instances to create.  Some of these instances may be scheduled on the same nodes, but exactly this many instances will be created and included in the cluster.

### Network

Under the `network` field, a `NetworkSpec` can be specified that describes network related settings of the cluster.
The properties that are currently supported are:

* `ports`: The port numbers to expose the CockroachDB services on, as shown in the [sample](#sample) above.  The supported port names are:
  * `http`: The port to bind to for HTTP requests such as the UI as well as health and debug endpoints.
  * `grpc`: The main port, served by gRPC, serves Postgres-flavor SQL, internode traffic and the command line interface.