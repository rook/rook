---
title: Examples
weight: 2050
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Ceph Examples

Configuration for Rook and Ceph can be configured in multiple ways to provide block devices, shared filesystem volumes or object storage in a kubernetes namespace. We have provided several examples to simplify storage setup, but remember there are many tunables and you will need to decide what settings work for your use case and environment.

See the **[example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph)** folder for all the rook/ceph setup example spec files.

## Common Resources

The first step to deploy Rook is to create the common resources. The configuration for these resources will be the same for most deployments.
The [common.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/common.yaml) sets these resources up.

```console
kubectl create -f common.yaml
```

The examples all assume the operator and all Ceph daemons will be started in the same namespace. If you want to deploy the operator in a separate namespace, see the comments throughout `common.yaml`.

## Operator

After the common resources are created, the next step is to create the Operator deployment. Several spec file examples are provided in [this directory](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/):

* `operator.yaml`: The most common settings for production deployments
  * `kubectl create -f operator.yaml`
* `operator-openshift.yaml`: Includes all of the operator settings for running a basic Rook cluster in an OpenShift environment. You will also want to review the [OpenShift Prerequisites](ceph-openshift.md) to confirm the settings.
  * `oc create -f operator-openshift.yaml`

Settings for the operator are configured through environment variables on the operator deployment. The individual settings are documented in [operator.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/operator.yaml).

## Cluster CRD

Now that your operator is running, let's create your Ceph storage cluster:

* `cluster.yaml`: This file contains common settings for a production storage cluster. Requires at least three nodes.
* `cluster-test.yaml`: Settings for a test cluster where redundancy is not configured. Requires only a single node.
* `cluster-minimal.yaml`: Brings up a cluster with only one [ceph-mon](http://docs.ceph.com/docs/nautilus/man/8/ceph-mon/) and a [ceph-mgr](http://docs.ceph.com/docs/nautilus/mgr/) so the Ceph dashboard can be used for the remaining cluster configuration.

See the [Cluster CRD](ceph-cluster-crd.md) topic for more details on the settings.

Monitors may be configured to run on PVC storage. Details on [how to set this up
and some minor restrctions are described here](ceph-cluster-crd#mon-settings).

## Setting up consumable storage

Now we are ready to setup [block](https://ceph.com/ceph-storage/block-storage/), [shared filesystem](https://ceph.com/ceph-storage/file-system/) or [object storage](https://ceph.com/ceph-storage/object-storage/) in the Rook Ceph cluster. These kinds of storage are respectively referred to as CephBlockPool, CephFilesystem and CephObjectStore in the spec files.

### Block Devices

Ceph can provide raw block device volumes to pods. Each example below sets up a storage class which can then be used to provision a block device in kubernetes pods. The storage class is defined with [a pool](http://docs.ceph.com/docs/master/rados/operations/pools/) which defines the level of data redundancy in Ceph:

* `storageclass.yaml`: This example illustrates replication of 3 for production scenarios and requires at least three nodes. Your data is replicated on three different kubernetes worker nodes and intermittent or long-lasting single node failures will not result in data unavailability or loss.
* `storageclass-ec.yaml`: Configures erasure coding for data durability rather than replication. [Ceph's erasure coding](http://docs.ceph.com/docs/master/rados/operations/erasure-code/) is more efficient than replication so you can get high reliability without the 3x replication cost of the preceding example (but at the cost of higher computational encoding and decoding costs on the worker nodes). Erasure coding requires at least three nodes. See the [Erasure coding](ceph-pool-crd.md#erasure-coded) documentation for more details. **Note: Erasure coding is only available with the flex driver. Support from the CSI driver is coming soon.**
* `storageclass-test.yaml`: Replication of 1 for test scenarios and it requires only a single node. Do not use this for applications that store valuable data or have high-availability storage requirements, since a single node failure can result in data loss.

The storage classes are found in different sub-directories depending on the driver:

* `csi/rbd`: The CSI driver for block devices. This is the preferred driver going forward.
* `flex`: The flex driver will be deprecated in a future release to be determined.

See the [Ceph Pool CRD](ceph-pool-crd.md) topic for more details on the settings.

### Shared Filesystem

Ceph filesystem (CephFS) allows the user to 'mount' a shared posix-compliant folder into one or more hosts (pods in the container world). This storage is similar to NFS shared storage or CIFS shared folders, as explained [here](https://ceph.com/ceph-storage/file-system/).

File storage contains multiple pools that can be configured for different scenarios:

* `filesystem.yaml`: Replication of 3 for production scenarios. Requires at least three nodes.
* `filesystem-ec.yaml`: Erasure coding for production scenarios. Requires at least three nodes.
* `filesystem-test.yaml`: Replication of 1 for test scenarios. Requires only a single node.

Dynamic provisioning is possible with the CSI driver. The storage class for shared filesystems is found in the `csi/cephfs` directory.

See the [Shared Filesystem CRD](ceph-filesystem-crd.md) topic for more details on the settings.

### Object Storage

Ceph supports storing blobs of data called objects that support HTTP(s)-type get/put/post and delete semantics. This storage is similar to AWS S3 storage, for example.

Object storage contains multiple pools that can be configured for different scenarios:

* `object.yaml`: Replication of 3 for production scenarios.  Requires at least three nodes.
* `object-openshift.yaml`: Replication of 3 with rgw in a port range valid for OpenShift.  Requires at least three nodes.
* `object-ec.yaml`: Erasure coding rather than replication for production scenarios.  Requires at least three nodes.
* `object-test.yaml`: Replication of 1 for test scenarios. Requires only a single node.

See the [Object Store CRD](ceph-object-store-crd.md) topic for more details on the settings.

### Object Storage User

* `object-user.yaml`: Creates a simple object storage user and generates credentials for the S3 API

### Object Storage Buckets

The Ceph operator also runs an object store bucket provisioner which can grant access to existing buckets or dynamically provision new buckets.

* [object-bucket-claim-retain.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/object-bucket-claim-retain.yaml) Creates a request for a new bucket by referencing a StorageClass which saves the bucket when the initiating OBC is deleted.
* [object-bucket-claim-delete.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/object-bucket-claim-delete.yaml) Creates a request for a new bucket by referencing a StorageClass which deletes the bucket when the initiating OBC is deleted.
* [storageclass-bucket-retain.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/storageclass-bucket-retain.yaml) Creates a new StorageClass which defines the Ceph Object Store, a region, and retains the bucket after the initiating OBC is deleted.
* [storageclass-bucket-delete.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/storageclass-bucket-delete.yaml) Creates a new StorageClass which defines the Ceph Object Store, a region, and deletes the bucket after the initiating OBC is deleted.
