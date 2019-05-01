---
title: Ceph Storage
weight: 300
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Ceph Storage Quickstart

This guide will walk you through the basic setup of a Ceph cluster and enable you to consume block, object, and file storage
from other pods running in your cluster.

## Minimum Version

Kubernetes **v1.10** or higher is supported by Rook.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

## TL;DR

If you're feeling lucky, a simple Rook cluster can be created with the following kubectl commands and [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph). For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).
```
cd cluster/examples/kubernetes/ceph
kubectl create -f common.yaml
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod. Check that you are using the [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph) that correspond to your release of Rook. For more options, see the [examples documentation](ceph-examples.md).

```bash
cd cluster/examples/kubernetes/ceph
kubectl create -f common.yaml
kubectl create -f operator.yaml

# verify the rook-ceph-operator, rook-ceph-agent, and rook-discover pods are in the `Running` state before proceeding
kubectl -n rook-ceph get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

## Create a Rook Ceph Cluster

Now that the Rook operator, agent, and discover pods are running, we can create the Rook Ceph cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property that is valid for your hosts. For more settings, see the documentation on [configuring the cluster](ceph-cluster-crd.md).


Save the cluster spec as `cluster.yaml`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # For the latest ceph images, see https://hub.docker.com/r/ceph/ceph/tags
    image: ceph/ceph:v14.2.1-20190430
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  dashboard:
    enabled: true
  storage:
    useAllNodes: true
    useAllDevices: true
```

Create the cluster:

```bash
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook-ceph` namespace. You should be able to see the following pods once they are all running.
The number of osd pods will depend on the number of nodes in the cluster and the number of devices and directories configured.

```bash
$ kubectl -n rook-ceph get pod
NAME                                   READY   STATUS      RESTARTS   AGE
rook-ceph-agent-4zkg8                  1/1     Running     0          140s
rook-ceph-mgr-a-d9dcf5748-5s9ft        1/1     Running     0          77s
rook-ceph-mon-a-7d8f675889-nw5pl       1/1     Running     0          105s
rook-ceph-mon-b-856fdd5cb9-5h2qk       1/1     Running     0          94s
rook-ceph-mon-c-57545897fc-j576h       1/1     Running     0          85s
rook-ceph-operator-6c49994c4f-9csfz    1/1     Running     0          141s
rook-ceph-osd-0-7cbbbf749f-j8fsd       1/1     Running     0          25s
rook-ceph-osd-1-7f67f9646d-44p7v       1/1     Running     0          25s
rook-ceph-osd-2-6cd4b776ff-v4d68       1/1     Running     0          25s
rook-ceph-osd-prepare-minikube-vx2rz   0/2     Completed   0          60s
rook-discover-dhkb8                    1/1     Running     0          140s
```

# Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:
- **[Block](ceph-block.md)**: Create block storage to be consumed by a pod
- **[Object](ceph-object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster
- **[Shared File System](ceph-filesystem.md)**: Create a file system to be shared across multiple pods

# Ceph Dashboard

Ceph has a dashboard in which you can view the status of your cluster. Please see the [dashboard guide](ceph-dashboard.md) for more details.

# Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](ceph-toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

# Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./ceph-monitoring.md).

# Teardown

When you are done with the test cluster, see [these instructions](ceph-teardown.md) to clean up the cluster.
