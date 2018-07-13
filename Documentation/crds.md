---
title: Custom Resources
weight: 30
---

# Custom Resources (CRDs)

Rook allows you to create and manage your storage cluster through custom resource definitions (CRDs). Each type of resource
has its own CRD defined.

## Ceph
- [Cluster](ceph-cluster-crd.md): A Rook cluster provides the basis of the storage platform to serve block, object stores, and shared file systems.
- [Pool](ceph-pool-crd.md): A pool manages the backing store for a block store. Pools are also used internally by object and file stores.
- [Object Store](ceph-object-store-crd.md): An object store exposes storage with an S3-compatible interface.
- [File System](ceph-filesystem-crd.md): A file system provides shared storage for multiple Kubernetes pods.

## CockroachDB
- [Cluster](cockroachdb-cluster-crd.md): CockroachDB is an open-source distributed SQL database that is highly scalable across multiple global regions and also highly durable.

## Minio
- [Object Store](minio-object-store-crd.md): Minio is a high performance distributed object storage server, designed for large-scale private cloud infrastructure.