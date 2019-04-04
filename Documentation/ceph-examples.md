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

Configuration for Rook and Ceph comes in many shapes and sizes. While we have done everything possible to make configuring storage
simple, you will need to decide what settings work for your environment. The settings on the operator and the
Rook Custom Resource Definitions (CRDs) are flexible. To get started, we have created several common configurations.

See the **[Ceph examples][example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph)** folder for the yaml files.

## Common Resources
The first step to deploy Rook is to create the common resources. The configuration for these resources will be the same for most deployments
```
kubectl create -f common.yaml
```

The examples all assume the operator and all Ceph daemons will be started in the same namespace. If you want to deploy the operator in a separate namespace, see the comments throughout `common.yaml`.

## Operator

After the common resources are created, the next step is to create the Operator deployment. There are several examples provided:
- `operator.yaml`: The most common settings for production deployments
   - `kubectl create -f operator.yaml`
- `operator-openshift.yaml`: Includes all of the operator settings for running a basic Rook cluster in an OpenShift environment. You will also want to review the [OpenShift Prerequisites](openshift.md) to confirm the settings.
   - `oc create -f operator-openshift.yaml`
- `operator-with-csi.yaml`: Includes configuration for testing ceph-csi while the integration is still in progress. See the [CSI Drivers](ceph-csi-drivers.md) topic for more details.
   - `kubectl create -f operator-with-csi.yaml`

Settings for the operator are configured through environment variables on the operator deployment. The individual settings are documented in `operator.yaml`.

## Cluster CRD
Now that your operator is running, let's create your Ceph storage cluster:
- `cluster.yaml`: Common settings for a production storage cluster. Requires at least three nodes.
- `cluster-test.yaml`: Settings for a test cluster where redundancy is not configured. Requires only a single node.
- `cluster-minimal.yaml`: Brings up a cluster with only one mon and a mgr so the Ceph dashboard can be used for the remaining cluster configuration.

See the [Cluster CRD](ceph-cluster-crd.md) topic for more details on the settings.

## Storage Class
The storage class is defined with a pool which defines the level of data redundancy:
- `storageclass.yaml`: Replication of 3 for production scenarios. Requires at least three nodes.
- `storageclass-ec.yaml`: Configures erasure coding for data durability rather than replication. See the [Erasure coding](ceph-pool-crd.md#erasure-coded) documentation for more details. Requires at least three nodes.
- `storageclass-test.yaml`: Replication of 1 for test scenarios. Requires only a single node.

See the [Ceph Pool CRD](ceph-pool-crd.md) topic for more details on the settings.

## Shared File System
File storage contains multiple pools that can be configured for different scenarios:
- `filesystem.yaml`: Replication of 3 for production scenarios. Requires at least three nodes.
- `filesystem-ec.yaml`: Erasure coding for production scenarios. Requires at least three nodes.
- `filesystem-test.yaml`: Replication of 1 for test scenarios. Requires only a single node.

See the [Shared File System CRD](ceph-filesystem-crd.md) topic for more details on the settings.

## Object Storage
Object storage contains multiple pools that can be configured for different scenarios:
- `object.yaml`: Replication of 3 for production scenarios.  Requires at least three nodes.
- `object-openshift.yaml`: Replication of 3 with rgw in a port range valid for OpenShift.  Requires at least three nodes.
- `object-ec.yaml`: Erasure coding rather than replication for production scenarios.  Requires at least three nodes.
- `object-test.yaml`: Replication of 1 for test scenarios. Requires only a single node.

See the [Object Store CRD](ceph-object-store-crd.md) topic for more details on the settings.

### Object Storage User
- `object-user.yaml`: Creates a simple object storage user and generates creds for the S3 API
