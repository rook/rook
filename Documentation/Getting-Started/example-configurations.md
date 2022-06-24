---
title: Example Configurations
---

Configuration for Rook and Ceph can be configured in multiple ways to provide block devices, shared filesystem volumes or object storage in a kubernetes namespace. We have provided several examples to simplify storage setup, but remember there are many tunables and you will need to decide what settings work for your use case and environment.

See the **[example yaml files](https://github.com/rook/rook/blob/master/deploy/examples)** folder for all the rook/ceph setup example spec files.

## Common Resources

The first step to deploy Rook is to create the CRDs and other common resources. The configuration for these resources will be the same for most deployments.
The [crds.yaml](https://github.com/rook/rook/blob/master/deploy/examples/crds.yaml) and
[common.yaml](https://github.com/rook/rook/blob/master/deploy/examples/common.yaml) sets these resources up.

```console
kubectl create -f crds.yaml -f common.yaml
```

The examples all assume the operator and all Ceph daemons will be started in the same namespace. If you want to deploy the operator in a separate namespace, see the comments throughout `common.yaml`.

## Operator

After the common resources are created, the next step is to create the Operator deployment. Several spec file examples are provided in [this directory](https://github.com/rook/rook/blob/master/deploy/examples/):

* [`operator.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/operator.yaml): The most common settings for production deployments
  * `kubectl create -f operator.yaml`
* [`operator-openshift.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/operator-openshift.yaml): Includes all of the operator settings for running a basic Rook cluster in an OpenShift environment. You will also want to review the [OpenShift Prerequisites](../Getting-Started/ceph-openshift.md) to confirm the settings.
  * `oc create -f operator-openshift.yaml`

Settings for the operator are configured through environment variables on the operator deployment. The individual settings are documented in [operator.yaml](https://github.com/rook/rook/blob/master/deploy/examples/operator.yaml).

## Cluster CRD

Now that your operator is running, let's create your Ceph storage cluster. This CR contains the most critical settings
that will influence how the operator configures the storage. It is important to understand the various ways to configure
the cluster. These examples represent a very small set of the different ways to configure the storage.

* [`cluster.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster.yaml): This file contains common settings for a production storage cluster. Requires at least three worker nodes.
* [`cluster-test.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster-test.yaml): Settings for a test cluster where redundancy is not configured. Requires only a single node.
* [`cluster-on-pvc.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster-on-pvc.yaml): This file contains common settings for backing the Ceph Mons and OSDs by PVs. Useful when running in cloud environments or where local PVs have been created for Ceph to consume.
* [`cluster-external.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster-external.yaml): Connect to an [external Ceph cluster](../CRDs/Cluster/ceph-cluster-crd.md#external-cluster) with minimal access to monitor the health of the cluster and connect to the storage.
* [`cluster-external-management.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster-external-management.yaml): Connect to an [external Ceph cluster](../CRDs/Cluster/ceph-cluster-crd.md#external-cluster) with the admin key of the external cluster to enable
  remote creation of pools and configure services such as an [Object Store](../Storage-Configuration/Object-Storage-RGW/object-storage.md) or a [Shared Filesystem](../Storage-Configuration/Shared-Filesystem-CephFS/filesystem-storage.md).
* [`cluster-stretched.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster-stretched.yaml): Create a cluster in "stretched" mode, with five mons stretched across three zones, and the OSDs across two zones. See the [Stretch documentation](../CRDs/Cluster/ceph-cluster-crd.md#stretch-cluster).

See the [Cluster CRD](../CRDs/Cluster/ceph-cluster-crd.md) topic for more details and more examples for the settings.

## Setting up consumable storage

Now we are ready to setup [block](https://ceph.com/ceph-storage/block-storage/), [shared filesystem](https://ceph.com/ceph-storage/file-system/) or [object storage](https://ceph.com/ceph-storage/object-storage/) in the Rook Ceph cluster. These kinds of storage are respectively referred to as CephBlockPool, CephFilesystem and CephObjectStore in the spec files.

### Block Devices

Ceph can provide raw block device volumes to pods. Each example below sets up a storage class which can then be used to provision a block device in kubernetes pods. The storage class is defined with [a pool](http://docs.ceph.com/docs/master/rados/operations/pools/) which defines the level of data redundancy in Ceph:

* [`storageclass.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/storageclass.yaml): This example illustrates replication of 3 for production scenarios and requires at least three worker nodes. Your data is replicated on three different kubernetes worker nodes and intermittent or long-lasting single node failures will not result in data unavailability or loss.
* [`storageclass-ec.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/storageclass-ec.yaml): Configures erasure coding for data durability rather than replication. [Ceph's erasure coding](http://docs.ceph.com/docs/master/rados/operations/erasure-code/) is more efficient than replication so you can get high reliability without the 3x replication cost of the preceding example (but at the cost of higher computational encoding and decoding costs on the worker nodes). Erasure coding requires at least three worker nodes. See the [Erasure coding](../CRDs/Block-Storage/ceph-block-pool-crd.md#erasure-coded) documentation for more details.
* [`storageclass-test.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/storageclass-test.yaml): Replication of 1 for test scenarios and it requires only a single node. Do not use this for applications that store valuable data or have high-availability storage requirements, since a single node failure can result in data loss.

The storage classes are found in different sub-directories depending on the driver:

* `csi/rbd`: The CSI driver for block devices. This is the preferred driver going forward.

See the [Ceph Pool CRD](../CRDs/Block-Storage/ceph-block-pool-crd.md) topic for more details on the settings.

### Shared Filesystem

Ceph filesystem (CephFS) allows the user to 'mount' a shared posix-compliant folder into one or more hosts (pods in the container world). This storage is similar to NFS shared storage or CIFS shared folders, as explained [here](https://ceph.com/ceph-storage/file-system/).

File storage contains multiple pools that can be configured for different scenarios:

* [`filesystem.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/filesystem.yaml): Replication of 3 for production scenarios. Requires at least three worker nodes.
* [`filesystem-ec.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/filesystem-ec.yaml): Erasure coding for production scenarios. Requires at least three worker nodes.
* [`filesystem-test.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/filesystem-test.yaml): Replication of 1 for test scenarios. Requires only a single node.

Dynamic provisioning is possible with the CSI driver. The storage class for shared filesystems is found in the [`csi/cephfs`](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs) directory.

See the [Shared Filesystem CRD](../CRDs/Shared-Filesystem/ceph-filesystem-crd.md) topic for more details on the settings.

### Object Storage

Ceph supports storing blobs of data called objects that support HTTP(s)-type get/put/post and delete semantics. This storage is similar to AWS S3 storage, for example.

Object storage contains multiple pools that can be configured for different scenarios:

* [`object.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object.yaml): Replication of 3 for production scenarios.  Requires at least three worker nodes.
* [`object-openshift.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object-openshift.yaml): Replication of 3 with rgw in a port range valid for OpenShift. Requires at least three worker nodes.
* [`object-ec.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object-ec.yaml): Erasure coding rather than replication for production scenarios. Requires at least three worker nodes.
* [`object-test.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object-test.yaml): Replication of 1 for test scenarios. Requires only a single node.

See the [Object Store CRD](../CRDs/Object-Storage/ceph-object-store-crd.md) topic for more details on the settings.

### Object Storage User

* [`object-user.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object-user.yaml): Creates a simple object storage user and generates credentials for the S3 API

### Object Storage Buckets

The Ceph operator also runs an object store bucket provisioner which can grant access to existing buckets or dynamically provision new buckets.

* [object-bucket-claim-retain.yaml](https://github.com/rook/rook/blob/master/deploy/examples/object-bucket-claim-retain.yaml) Creates a request for a new bucket by referencing a StorageClass which saves the bucket when the initiating OBC is deleted.
* [object-bucket-claim-delete.yaml](https://github.com/rook/rook/blob/master/deploy/examples/object-bucket-claim-delete.yaml) Creates a request for a new bucket by referencing a StorageClass which deletes the bucket when the initiating OBC is deleted.
* [storageclass-bucket-retain.yaml](https://github.com/rook/rook/blob/master/deploy/examples/storageclass-bucket-retain.yaml) Creates a new StorageClass which defines the Ceph Object Store and retains the bucket after the initiating OBC is deleted.
* [storageclass-bucket-delete.yaml](https://github.com/rook/rook/blob/master/deploy/examples/storageclass-bucket-delete.yaml) Creates a new StorageClass which defines the Ceph Object Store and deletes the bucket after the initiating OBC is deleted.
