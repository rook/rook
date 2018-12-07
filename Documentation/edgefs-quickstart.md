---
title: EdgeFS Geo-Transparent Storage
weight: 7
indent: true
---

# EdgeFS Geo-Transparent Storage Quickstart

This guide will walk you through the basic setup of a EdgeFS cluster namespaces and enable you to consume S3 object, NFS file access, and iSCSI block storage
from other pods running in your cluster, in Geo-Transparent and distributed ways.

## Minimum Version

EdgeFS operator, CSI plugin and CRDs were tested with Kubernetes **v1.11** or higher.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

## TL;DR

If you're feeling lucky, a simple EdgeFS Rook cluster can be created with the following kubectl commands. For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).
```
cd cluster/examples/kubernetes/edgefs
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [NFS, S3, or iSCSI](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod.

Note that Google Cloud users need to explicitly grant user permission to create roles:

```bash
kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user $(gcloud config get-value account)
```

Now you ready to create operator:

```bash
cd cluster/examples/kubernetes/edgefs
kubectl create -f operator.yaml

# verify the rook-edgefs-operator, and rook-discover pods are in the `Running` state before proceeding
kubectl -n rook-edgefs-system get pod
```

You can also deploy the operator with the [Rook EdgeFS Helm Chart](edgefs-helm-operator.md).

## Create a Rook Cluster

Now that the Rook operator, and discover pods are running, we can create the Rook cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](edgefs-cluster-crd.md).


Edit the cluster spec in `cluster.yaml` file.

Create the cluster:

```bash
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook` namespace. You should be able to see the following pods once they are all running.
The number of osd pods will depend on the number of nodes in the cluster and the number of devices and directories configured.

```bash
$ kubectl -n rook-edgefs get pod
rook-edgefs          rook-edgefs-mgr-7c76cb564d-56sxb        1/1     Running   0          24s
rook-edgefs          rook-edgefs-target-0                    3/3     Running   0          24s
rook-edgefs          rook-edgefs-target-1                    3/3     Running   0          24s
rook-edgefs          rook-edgefs-target-2                    3/3     Running   0          24s
```

Notice that EdgeFS Targets are running as StatefulSet.

# Storage

For a walkthrough of the types of Storage CRDs exposed by EdgeFS Rook, see the guides for:
- **[NFS Server](edgefs-nfs-crd.md)**: Create Scale-Out NFS storage to be consumed by multiple pods
- **[S3X](edgefs-s3x-crd.md)**: Create an Extended S3 HTTP/2 compatible object and key-value store that is accessible inside or outside the Kubernetes cluster
- **[AWS S3](edgefs-s3-crd.md)**: Create an AWS S3 compatible object store that is accessible inside or outside the Kubernetes cluster
- **[iSCSI Target](edgefs-iscsi-crd.md)**: Create low-latency and high-performance iSCSI block to be consumed by a pod

# CSI Integration

EdgeFS comes with built-in gRPC management services, which are tightly integrated into Kubernetes provisioning and attaching CSI framework. Please see the [EdgeFS CSI](edgefs-csi.md) for setup and usage information.

# EdgeFS Dashboard and Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
In additional to classic monitoring parameters, it has a built-in multi-tenancy, multi-service capability. For instance, you can quickly discover most loaded tenants, buckets or services.
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./edgefs-monitoring.md).

# Teardown

When you are done with the test cluster, see [these instructions](edgefs-teardown.md) to clean up the cluster.
