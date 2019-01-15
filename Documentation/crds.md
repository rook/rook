---
title: Custom Resources
weight: 9
indent: true
---

# Custom Resources (CRDs)

Rook allows you to create and manage your storage cluster through custom resource definitions (CRDs). Each type of resource
has its own CRD defined.

## Ceph
- [Cluster](ceph-cluster-crd.md): A Rook cluster provides the basis of the storage platform to serve block, object stores, and shared file systems.
- [Block Pool](ceph-pool-crd.md): A pool manages the backing store for a block store.
- [Object Store](ceph-object-store-crd.md): An object store exposes storage with an S3-compatible interface.
- [Object Store User](ceph-object-store-user-crd.md): An object store user manages creation of S3 user credentials to access an object store.
- [File System](ceph-filesystem-crd.md): A file system provides shared storage for multiple Kubernetes pods.
- [NFS](ceph-nfs-crd.md): Expose the Ceph file system or object store via NFS.

## CockroachDB
- [Cluster](cockroachdb-cluster-crd.md): CockroachDB is an open-source distributed SQL database that is highly scalable across multiple global regions and also highly durable.

## Minio
- [Object Store](minio-object-store-crd.md): Minio is a high performance distributed object storage server, designed for large-scale private cloud infrastructure.

## Cassandra / Scylla
- [Cluster](cassandra-cluster-crd.md): [Cassandra](http://cassandra.apache.org/) is highly available, fault tolerant, peer-to-peer database featuring lightning fast performance and tunable consistency. It provides massive scalability with no single point of failure.
[Scylla](https://www.scylladb.com) is a close-to-the-hardware rewrite of Cassandra in C++. The rook operator supports both.
