---
title: Ceph Storage
weight: 300
indent: true
---

{% include_relative branch.liquid %}


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

```console
git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd cluster/examples/kubernetes/ceph
kubectl create -f common.yaml
kubectl create -f operator.yaml
kubectl create -f cluster-test.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

### Production Environments

For production environments it is required to have local storage devices attached to your nodes.
In this walkthrough, the requirement of local storage devices is relaxed so you can get a cluster up and running
as a "test" environment to experiment with Rook. A Ceph filestore OSD will be created in a `directory` instead
of requiring a device. For production environments, you will want to follow the example in `cluster.yaml` instead of
`cluster-test.yaml` in order to configure the devices instead of test directories. See the [Ceph examples](ceph-examples.md) for more details.

## Deploy the Rook Operator

The first step is to deploy the Rook operator. Check that you are using the [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph) that correspond to your release of Rook. For more options, see the [examples documentation](ceph-examples.md).

```console
cd cluster/examples/kubernetes/ceph
kubectl create -f common.yaml
kubectl create -f operator.yaml

## verify the rook-ceph-operator is in the `Running` state before proceeding
kubectl -n rook-ceph get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

## Create a Rook Ceph Cluster

Now that the Rook operator is running we can create the Ceph cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property that is valid for your hosts. For more settings, see the documentation on [configuring the cluster](ceph-cluster-crd.md).

Save the cluster spec as `cluster-test.yaml`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # For the latest ceph images, see https://hub.docker.com/r/ceph/ceph/tags
    image: ceph/ceph:v14.2.6
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
  dashboard:
    enabled: true
  storage:
    useAllNodes: true
    useAllDevices: false
    # Important: Directories should only be used in pre-production environments
    directories:
    - path: /var/lib/rook

```

Create the cluster:

```console
kubectl create -f cluster-test.yaml
```

Use `kubectl` to list pods in the `rook-ceph` namespace. You should be able to see the following pods once they are all running.
The number of osd pods will depend on the number of nodes in the cluster and the number of devices and directories configured.
If you did not modify the `cluster-test.yaml` above, it is expected that one OSD will be created per node.
The `rook-ceph-agent` and `rook-discover` pods are also optional depending on your settings.

```console
$ kubectl -n rook-ceph get pod
NAME                                   READY   STATUS      RESTARTS   AGE
rook-ceph-agent-4zkg8                  1/1     Running     0          140s
rook-ceph-mgr-a-d9dcf5748-5s9ft        1/1     Running     0          77s
rook-ceph-mon-a-7d8f675889-nw5pl       1/1     Running     0          105s
rook-ceph-mon-b-856fdd5cb9-5h2qk       1/1     Running     0          94s
rook-ceph-mon-c-57545897fc-j576h       1/1     Running     0          85s
rook-ceph-operator-6c49994c4f-9csfz    1/1     Running     0          141s
rook-ceph-osd-0-7cbbbf749f-j8fsd       1/1     Running     0          23s
rook-ceph-osd-1-7f67f9646d-44p7v       1/1     Running     0          24s
rook-ceph-osd-2-6cd4b776ff-v4d68       1/1     Running     0          25s
rook-ceph-osd-prepare-node1-vx2rz      0/2     Completed   0          60s
rook-ceph-osd-prepare-node2-ab3fd      0/2     Completed   0          60s
rook-ceph-osd-prepare-node3-w4xyz      0/2     Completed   0          60s
rook-discover-dhkb8                    1/1     Running     0          140s
```

To verify that the cluster is in a healthy state, connect to the [Rook toolbox](ceph-toolbox.md) and run the
`ceph status` command.

* All mons should be in quorum
* A mgr should be active
* At least one OSD should be active
* If the health is not `HEALTH_OK`, the warnings or errors should be investigated

```console
$ ceph status
  cluster:
    id:     a0452c76-30d9-4c1a-a948-5d8405f19a7c
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum a,b,c (age 3m)
    mgr: a(active, since 2m)
    osd: 3 osds: 3 up (since 1m), 3 in (since 1m)
...
```

If the cluster is not healthy, please refer to the [Ceph common issues](ceph-common-issues.md) for more details and potential solutions.

## Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:

* **[Block](ceph-block.md)**: Create block storage to be consumed by a pod
* **[Object](ceph-object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster
* **[Shared Filesystem](ceph-filesystem.md)**: Create a filesystem to be shared across multiple pods

## Ceph Dashboard

Ceph has a dashboard in which you can view the status of your cluster. Please see the [dashboard guide](ceph-dashboard.md) for more details.

## Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](ceph-toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

## Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./ceph-monitoring.md).

## Teardown

When you are done with the test cluster, see [these instructions](ceph-teardown.md) to clean up the cluster.
