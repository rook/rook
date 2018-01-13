---
title: Custom Resources
weight: 30
---

# Custom Resources (CRDs)

Rook allows you to create and manage your storage cluster through custom resource definitions (CRDs). Each type of resource
has its own CRD defined.
- [Cluster](cluster-crd.md): A Rook cluster provides the basis of the storage platform to serve block, object stores, and shared file systems.
- [Pool](pool-crd.md): A pool manages the backing store for a block store. Pools are also used internally by object and file stores.
- [Object Store](object-store-crd.md): An object store exposes storage with an S3-compatible interface.
- [File System](filesystem-crd.md): A file system provides shared storage for multiple Kubernetes pods.