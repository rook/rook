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

## EdgeFS
- [Cluster](edgefs-cluster-crd.md): EdgeFS is high-performance and low-latency object storage system with Geo-Transparent data access from on-prem, private/public clouds or small footprint edge (IoT) devices
- [NFS](edgefs-nfs-crd.md): POSIX compliant Scale-Out NFS with file-level granularity snapshots and global data deduplication
- [S3X](edgefs-s3x-crd.md): Extreamly low-latency, high-performance S3 compatible API designed for AI/ML workloads with ranged-writes capability, NOSQL record store and HTTP/2 streaming operations
- [ISCSI](edgefs-iscsi-crd.md): iSCSI interface to EdgeFS object(s) presented as block device
- [ISGW](edgefs-isgw-crd.md): EdgeFS Geo-site interconnect link, with support for bidirectional and many-to-many synchronizations over secured TCP/IP channel(s)
