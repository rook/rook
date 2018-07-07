---
title: Quickstart
weight: 2
---

# Quickstart Guide

Welcome to Rook! We hope you have a great experience installing the Rook storage platform to enable highly available, durable storage
in your cluster. If you have any questions along the way, please don't hesitate to ask us in our [Slack channel](https://rook-io.slack.com). Sign up for our Slack [here](https://rook-slackin.herokuapp.com/).

This guide will walk you through the basic setup of a Rook cluster. This will enable you to consume block, object, and file storage
from other pods running in your cluster.

## Minimum Version

Kubernetes **v1.7** or higher is supported by Rook.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

## TL;DR

If you're feeling lucky, a simple Rook cluster can be created with the following kubectl commands. For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).
```
cd cluster/examples/kubernetes/ceph
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod.

```bash
cd cluster/examples/kubernetes/ceph
kubectl create -f operator.yaml

# verify the rook-ceph-operator and rook-ceph-agent pods are in the `Running` state before proceeding
kubectl -n rook-ceph-system get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

---
### **Restart Kubelet**
**(K8S 1.7.x only)**

For versions of Kubernetes prior to 1.8, the Kubelet process on all nodes will require a restart after the Rook operator and Rook agents have been deployed. As part of their initial setup, the Rook agents deploy and configure a Flexvolume plugin in order to integrate with Kubernetes' volume controller framework. In Kubernetes v1.8+, the [dynamic Flexvolume plugin discovery](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#dynamic-plugin-discovery) will find and initialize our plugin, but in older versions of Kubernetes a manual restart of the Kubelet will be required.

---

## Create a Rook Cluster

Now that the Rook operator and agent pods are running, we can create the Rook cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](ceph-cluster-crd.md).


Save the cluster spec as `cluster.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
apiVersion: ceph.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  dashboard:
    enabled: true
  storage:
    useAllNodes: true
    useAllDevices: false
    config:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
```

Create the cluster:

```bash
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook` namespace. You should be able to see the following pods once they are all running:

```bash
$ kubectl -n rook-ceph get pod
NAME                              READY     STATUS    RESTARTS   AGE
rook-ceph-mgr0-1279756402-wc4vt   1/1       Running   0          5m
rook-ceph-mon0-jflt5              1/1       Running   0          6m
rook-ceph-mon1-wkc8p              1/1       Running   0          6m
rook-ceph-mon2-p31dj              1/1       Running   0          6m
rook-ceph-osd-0h6nb               1/1       Running   0          5m
```

# Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:
- **[Block](block.md)**: Create block storage to be consumed by a pod
- **[Object](object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster
- **[Shared File System](filesystem.md)**: Create a file system to be shared across multiple pods

# Ceph Dashboard

Ceph has a dashboard in which you can view the status of your cluster. Please see the [dashboard guide](ceph-dashboard.md) for more details.

# Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

# Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./monitoring.md).

# Teardown

When you are done with the test cluster, see [these instructions](teardown.md) to clean up the cluster.
