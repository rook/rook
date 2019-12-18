---
title: EdgeFS Data Fabric
weight: 550
indent: true
---

{% include_relative branch.liquid %}

# EdgeFS Data Fabric Quickstart

EdgeFS Data Fabric virtualzing common storage protocols and enables multi-cluster, multi-region data flow topologies.

This guide will walk you through the basic setup of a EdgeFS cluster namespaces and enable you to consume S3 object, NFS file access, and iSCSI block storage
from other pods running in your cluster, in decentralized ways.

## Minimum Version

EdgeFS operator, CSI plugin and CRDs were tested with Kubernetes **v1.11** or higher.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

A minimum of 3 storage devices are required with the default data replication count of 3. (To test EdgeFS on a single-node cluster with only one device or storage directory, set `sysRepCount` to `1` under the `rook-edgefs` `Cluster` object manifest in `cluster/examples/kubernetes/edgefs/cluster.yml`)

To operate efficiently EdgeFS requires 1 CPU core and 1GB of memory per storage device. Minimal memory requirement for EdgeFS target pod is 4GB. To get maximum out of SSD/NVMe device we recommend to double requirements to 2 CPU and 2GB per device.

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

We recommend you to configure EdgeFS to use of raw devices and equal distribution of available storage capacity.

> **IMPORTANT**: EdgeFS will automatically adjust deployment nodes to use larger then 128KB data chunks, with the following addition to /etc/sysctl.conf:

```ini
net.core.rmem_default = 80331648
net.core.rmem_max = 80331648
net.core.wmem_default = 33554432
net.core.wmem_max = 50331648
vm.dirty_ratio = 10
vm.dirty_background_ratio = 5
vm.swappiness = 15
```

To turn off this node adjustment need to enable `skipHostPrepare` option in cluster CRD [configuring the cluster](edgefs-cluster-crd.md).

## TL;DR

If you're feeling lucky, a simple EdgeFS Rook cluster can be created with the following kubectl commands. For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).

```console
git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd cluster/examples/kubernetes/edgefs
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [NFS, S3, or iSCSI](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod.

Note that Google Cloud users need to explicitly grant user permission to create roles:

```console
kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user $(gcloud config get-value account)
```

Now you ready to create operator:

```console
cd cluster/examples/kubernetes/edgefs
kubectl create -f operator.yaml

# verify the rook-edgefs-operator, and rook-discover pods are in the `Running` state before proceeding
kubectl -n rook-edgefs-system get pod
```

## Create a Rook Cluster

Now that the Rook operator, and discover pods are running, we can create the Rook cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](edgefs-cluster-crd.md).

Edit the cluster spec in `cluster.yaml` file.

Create the cluster:

```console
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook-edgefs` namespace. You should be able to see the following pods once they are all running.
The number of target pods will depend on the number of nodes in the cluster and the number of devices and directories configured.

```console
$ kubectl -n rook-edgefs get pod
rook-edgefs          rook-edgefs-mgr-7c76cb564d-56sxb        1/1     Running   0          24s
rook-edgefs          rook-edgefs-target-0                    3/3     Running   0          24s
rook-edgefs          rook-edgefs-target-1                    3/3     Running   0          24s
rook-edgefs          rook-edgefs-target-2                    3/3     Running   0          24s
```

Notice that EdgeFS Targets are running as StatefulSet.

## Storage

For a walkthrough of the types of Storage CRDs exposed by EdgeFS Rook, see the guides for:

* **[NFS Server](edgefs-nfs-crd.md)**: Create Scale-Out NFS storage to be consumed by multiple pods, simultaneously
* **[S3X](edgefs-s3x-crd.md)**: Create an Extended S3 HTTP/2 compatible object and key-value store that is accessible inside or outside the Kubernetes cluster
* **[AWS S3](edgefs-s3-crd.md)**: Create an AWS S3 compatible object store that is accessible inside or outside the Kubernetes cluster
* **[iSCSI Target](edgefs-iscsi-crd.md)**: Create low-latency and high-throughput iSCSI block to be consumed by a pod

## CSI Integration

EdgeFS comes with built-in gRPC management services, which are tightly integrated into Kubernetes provisioning and attaching CSI framework. Please see the [EdgeFS CSI](edgefs-csi.md) for setup and usage information.

## EdgeFS Dashboard and Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
In additional to classic monitoring parameters, it has a built-in multi-tenancy, multi-service capability. For instance, you can quickly discover most loaded tenants, buckets or services.
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./edgefs-monitoring.md).

## Teardown

When you are done with the cluster, simply delete CRDs in reverse order. You may want to re-format your raw disks with `wipefs -a` command. Or if you using raw devices and want to keep same storage configuration but change some resource or networking parameters, consider to use `devicesResurrectMode`.
