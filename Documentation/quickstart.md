---
title: Quickstart
weight: 2
---

# Quickstart Guide

Welcome to Rook! We hope you have a great experience installing the Rook storage platform to enable highly available, durable storage
in your cluster. If you have any questions along the way, please don't hesitate to ask us in our [Slack channel](https://Rook-io.slack.com).

This guide will walk you through the basic setup of a Ceph cluster. This will enable you to consume block, object, and file storage
from other pods running in your cluster.

## Minimum Version

Kubernetes **v1.6** or higher is targeted by Rook (while Rook is in alpha it will track the latest release to use the latest features).

Support is available for Kubernetes **v1.5.2**, although your mileage may vary.
You will need to use the yaml files from the [1.5 folder](/cluster/examples/kubernetes/1.5).

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

## TL;DR

If you're feeling lucky, a simple Ceph cluster can be created with the following kubectl commands. For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).
```
cd cluster/examples/kubernetes
kubectl create -f rook-operator.yaml
kubectl create -f ceph-cluster.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod.

```bash
cd cluster/examples/kubernetes
kubectl create -f rook-operator.yaml

# verify the rook-operator and rook-agents pods are in the `Running` state before proceeding
kubectl -n rook-system get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

---
### **Restart Kubelet**
**(K8S 1.7.x and older only)**

For versions of Kubernetes prior to 1.8, the Kubelet process on all nodes will require a restart after the Rook operator and Rook agents have been deployed. As part of their initial setup, the Rook agents deploy and configure a Flexvolume plugin in order to integrate with Kubernetes' volume controller framework. In Kubernetes v1.8+, the [dynamic Flexvolume plugin discovery](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#dynamic-plugin-discovery) will find and initialize our plugin, but in older versions of Kubernetes a manual restart of the Kubelet will be required.

### **Disable Attacher-detacher controller**
**(K8S 1.6.x only)**

For Kubernetes 1.6, it is also necessary to pass the `--enable-controller-attach-detach=false` flag to Kubelet when you restart it.  This is a workaround for a [Kubernetes issue](https://github.com/kubernetes/kubernetes/issues/47109) that only affects 1.6.

---

## Create a Rook Cluster

Now that the Rook operator and agent pods are running, we can create the Ceph cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](cluster-crd.md).


Save the cluster spec as `ceph-cluster.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/rook
  storage:
    useAllNodes: true
    useAllDevices: false
    storeConfig:
      storeType: bluestore
      databaseSizeMB: 1024
      journalSizeMB: 1024
```

Create the cluster:

```bash
kubectl create -f rook-cluster.yaml
```

Use `kubectl` to list pods in the `ceph` namespace. You should be able to see the following pods once they are all running:

```bash
$ kubectl -n ceph get pod
NAME                              READY     STATUS    RESTARTS   AGE
rook-api-1511082791-7qs0m         1/1       Running   0          5m
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

# Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Ceph cluster.  Please see the [toolbox readme](toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

The toolbox also contains the `rookctl` tool as required in the [File System](filesystem.md) and [Object](object.md) walkthroughs, or a [simplified walkthrough of block, file and object storage](client.md). In the near future, `rookctl` will not be required for kubernetes scenarios.

# Monitoring

Each Ceph cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Ceph cluster, you can follow the steps in the [monitoring guide](./monitoring.md).

# Teardown

To clean up all the artifacts created by the demo, first cleanup the resources from the [block](block.md#teardown) and [file](filesystem.md#teardown) walkthroughs (unmount volumes, delete volume claims, etc).
Those steps have been copied below for convenience, but note that some of these may not exist if you did not complete those parts of the demo:
```console
kubectl delete -f wordpress.yaml
kubectl delete -f mysql.yaml
kubectl delete -n ceph pool replicapool
kubectl delete storageclass rook-block
kubectl -n kube-system delete secret rook-admin
kubectl delete -f kube-registry.yaml
```

After those resources have been cleaned up, you can then delete your Ceph cluster:
```console
kubectl delete -n ceph cluster rook
```

This will begin the process of all cluster resources being cleaned up, after which you can delete the rest of the deployment with the following:
```console
kubectl delete thirdpartyresources cluster.rook.io pool.rook.io objectstore.rook.io filesystem.rook.io volumeattachment.rook.io # ignore errors if on K8s 1.7+
kubectl delete crd clusters.rook.io pools.rook.io objectstores.rook.io filesystems.rook.io volumeattachments.rook.io  # ignore errors if on K8s 1.5 and 1.6
kubectl delete -n rook-system daemonset rook-agent
kubectl delete -f rook-operator.yaml
kubectl delete clusterroles rook-agent
kubectl delete clusterrolebindings rook-agent
kubectl delete namespace ceph
```

IMPORTANT: The final cleanup step requires deleting files on each host in the cluster. All files under the `dataDirHostPath` property specified in the cluster CRD will need to be deleted. Otherwise, inconsistent state will remain when a new cluster is started.

If you modified the demo settings, additional cleanup is up to you for devices, host paths, etc.
