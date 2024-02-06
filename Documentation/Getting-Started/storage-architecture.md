---
title: Storage Architecture
---

Ceph is a highly scalable distributed storage solution for **block storage**, **object storage**, and **shared filesystems** with years of production deployments.

## Design

Rook enables Ceph storage to run on Kubernetes using Kubernetes primitives.
With Ceph running in the Kubernetes cluster, Kubernetes applications can
mount block devices and filesystems managed by Rook, or can use the S3/Swift API for object storage. The Rook operator
automates configuration of storage components and monitors the cluster to ensure the storage remains available
and healthy.

The Rook operator is a simple container that has all that is needed to bootstrap
and monitor the storage cluster. The operator will start and monitor [Ceph monitor pods](../Storage-Configuration/Advanced/ceph-mon-health.md), the Ceph OSD daemons to provide RADOS storage, as well as start and manage other Ceph daemons. The operator manages CRDs for pools, object stores (S3/Swift), and filesystems by initializing the pods and other resources necessary to run the services.

The operator will monitor the storage daemons to ensure the cluster is healthy. Ceph mons will be started or failed over when necessary, and
other adjustments are made as the cluster grows or shrinks.  The operator will also watch for desired state changes
specified in the Ceph custom resources (CRs) and apply the changes.

Rook automatically configures the Ceph-CSI driver to mount the storage to your pods.
The `rook/ceph` image includes all necessary tools to manage the cluster. Rook is not in the Ceph data path.
Many of the Ceph concepts like placement groups and crush maps
are hidden so you don't have to worry about them. Instead, Rook creates a simplified user experience for admins that is in terms
of physical resources, pools, volumes, filesystems, and buckets. Advanced configuration can be applied when needed with the Ceph tools.

Rook is implemented in golang. Ceph is implemented in C++ where the data path is highly optimized. We believe
this combination offers the best of both worlds.

### Architecture

![Rook Components on Kubernetes](ceph-storage/Rook%20High-Level%20Architecture.png)

Example applications are shown above for the three supported storage types:

- Block Storage is represented with a blue app, which has a `ReadWriteOnce (RWO)` volume mounted. The application can read and write to the RWO volume, while Ceph manages the IO.
- Shared Filesystem is represented by two purple apps that are sharing a ReadWriteMany (RWX) volume. Both applications can actively read or write simultaneously to the volume. Ceph will ensure the data is safely protected for multiple writers with the MDS daemon.
- Object storage is represented by an orange app that can read and write to a bucket with a standard S3 client.

Below the dotted line in the above diagram, the components fall into three categories:

- Rook operator (blue layer): The operator automates configuration of Ceph
- CSI plugins and provisioners (orange layer): The Ceph-CSI driver provides the provisioning and mounting of volumes
- Ceph daemons (red layer): The Ceph daemons run the core storage architecture. See the [Glossary](glossary.md#ceph) to learn more about each daemon.

Production clusters must have three or more nodes for a resilient storage platform.

### Block Storage

In the diagram above, the flow to create an application with an RWO volume is:

- The (blue) app creates a PVC to request storage
- The PVC defines the Ceph RBD storage class (sc) for provisioning the storage
- K8s calls the Ceph-CSI RBD provisioner to create the Ceph RBD image.
- The kubelet calls the CSI RBD volume plugin to mount the volume in the app
- The volume is now available for reads and writes.

A ReadWriteOnce volume can be mounted on one node at a time.

### Shared Filesystem

In the diagram above, the flow to create a applications with a RWX volume is:

- The (purple) app creates a PVC to request storage
- The PVC defines the CephFS storage class (sc) for provisioning the storage
- K8s calls the Ceph-CSI CephFS provisioner to create the CephFS subvolume
- The kubelet calls the CSI CephFS volume plugin to mount the volume in the app
- The volume is now available for reads and writes.

A ReadWriteMany volume can be mounted on multiple nodes for your application to use.

### Object Storage S3

In the diagram above, the flow to create an application with access to an S3 bucket is:

- The (orange) app creates an ObjectBucketClaim (OBC) to request a bucket
- The Rook operator creates a Ceph RGW bucket (via the lib-bucket-provisioner)
- The Rook operator creates a secret with the credentials for accessing the bucket and a configmap with bucket information
- The app retrieves the credentials from the secret
- The app can now read and write to the bucket with an S3 client

A S3 compatible client can use the S3 bucket right away using the credentials (`Secret`) and bucket info (`ConfigMap`).
