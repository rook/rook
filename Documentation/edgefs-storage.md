---
title: EdgeFS Data Fabric
weight: 4000
---

# EdgeFS Data Fabric

[EdgeFS](http://edgefs.io) is high-performance and fault-tolerant decentralized data fabric with virtualized access to S3 object, NFS file, NoSQL and iSCSI block.

EdgeFS is capable of spanning unlimited number of geographically distributed sites (Geo-site), connected with each other as one global name space data fabric running on top of Kubernetes platform, providing persistent, fault-tolerant and high-performance volumes for stateful Kubernetes Applications.

At each Geo-site, EdgeFS nodes deployed as containers (StatefulSet) on physical or virtual Kubernetes nodes, pooling available storage capacity and presenting it via compatible S3/NFS/iSCSI/etc storage emulated protocols for cloud-native applications running on the same or dedicated servers.

## How it works, in a Nutshell?

If you familiar with "git", where all modifications are fully versioned and globally immutable, it is highly likely you already know how it works at its core. Think of it as a world-scale copy-on-write technique. Now, if we can make a parallel for you to understand it better - what EdgeFS does, it expands "git" paradigm to object storage and making Kubernetes Persistent Volumes accessible via emulated storage standard protocols e.g. S3, NFS and even block devices such as iSCSI, in a high-performance and low-latency ways. With fully versioned modifications, fully immutable metadata and data, users data can be transparently replicated, distributed and dynamically pre-fetched across many Geo-sites.

## Design

Rook enables easy deployment of EdgeFS Geo-sites on Kubernetes using Kubernetes primitives.

![EdgeFS Rook Architecture on Kubernetes](media/edgefs-rook.png)
With Rook running in the Kubernetes cluster, Kubernetes PODs or External applications can
mount block devices and filesystems managed by Rook, or can use the S3/S3X API for object storage. The Rook operator
automates configuration of storage components and monitors the cluster to ensure the storage remains available
and healthy.

The Rook operator is a simple container that has all that is needed to bootstrap and monitor the storage cluster. The operator will start and monitor StatefulSet storage Targets, gRPC manager and Prometheus Multi-Tenant Dashboard. All the attached devices (or directories) will provide pooled storage site. Storage sites then can be easily connected with each other as one global name space data fabric. The operator manages CRDs for Targets, Scale-out NFS, Object stores (S3/S3X), and iSCSI volumes by initializing the pods and other artifacts necessary to
run the services.

The operator will monitor the storage Targets to ensure the cluster is healthy. EdgeFS will dynamically handle services failover, and other adjustments that maybe made as the cluster grows or shrinks.

The EdgeFS Rook operator also comes with tightly integrated CSI plugin. CSI pods deployed on every Kubernetes node. All storage operations required on the node are handled such as attaching network storage devices, mounting NFS exports, and dynamic provisioning.

Rook is implemented in golang. EdgeFS is implemented in Go/C where the data path is highly optimized.

Learn more at [edgefs.io](http://edgefs.io).
