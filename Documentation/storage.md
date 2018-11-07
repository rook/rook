---
title: Ceph Storage
weight: 20
---

# Ceph Storage

Rook provides three types of storage to the Kubernetes cluster with the Ceph operator.
- [Block Storage](block.md): Mount storage to a single pod
- [Object Storage](object.md): Expose an S3 API to the storage cluster for applications to put and get data that is accessible from inside or outside the Kubernetes cluster
- [Shared File System](filesystem.md): Mount a file system that can be shared across multiple pods
