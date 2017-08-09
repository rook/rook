---
title: Kubernetes
weight: 10
---

# Rook on Kubernetes

- [Quickstart](#quickstart)
- [Design](#design)

## Quickstart

This example shows how to build a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

### Minimum Version

Kubernetes **v1.6** or higher is targeted by Rook (while Rook is in alpha it will track the latest release to use the latest features).

Support is available for Kubernetes **v1.5.2**, although your mileage may vary.
You will need to use the yaml files from the [1.5 folder](/demo/kubernetes/1.5).

### Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these quick instructions](k8s-pre-reqs.md), including:
- The `kubelet` requires access to `modprobe` and `rbd` on host

Note that we are striving for even more smooth integration with Kubernetes in the future such that `Rook` will work out of the box with any Kubernetes cluster.

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

### Deploy Rook

With your Kubernetes cluster running, Rook can be setup and deployed by simply creating the rook-operator deployment and creating a rook cluster. To customize the operator settings, see the [Operator Helm Chart](helm-operator.md).

```bash
cd demo/kubernetes
kubectl create -f rook-operator.yaml

# verify the rook-operator pod is in the `Running` state before proceeding
kubectl get pod
```

Now that the rook-operator pod is running, we can create the Rook cluster. For the cluster to survive reboots, 
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](cluster-crd.md). 

Save the cluster spec as `rook-cluster.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook
  namespace: rook
spec:
  versionTag: v0.5.0
  dataDirHostPath:
  storage:
    useAllNodes: true
    useAllDevices: false
    storeConfig:
      storeType: filestore
      databaseSizeMB: 1024
      journalSizeMB: 1024
```

Create the cluster:

```bash
kubectl create -f rook-cluster.yaml
```

Use `kubectl` to list pods in the `rook` namespace. You should be able to see the following pods once they are all running:

```bash
$ kubectl -n rook get pod
NAME                              READY     STATUS    RESTARTS   AGE
rook-api-1511082791-7qs0m         1/1       Running   0          5m
rook-ceph-mgr0-1279756402-wc4vt   1/1       Running   0          5m
rook-ceph-mon0-jflt5              1/1       Running   0          6m
rook-ceph-mon1-wkc8p              1/1       Running   0          6m
rook-ceph-mon2-p31dj              1/1       Running   0          6m
rook-ceph-osd-0h6nb               1/1       Running   0          5m
```

## Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:
- **[Block](k8s-block.md)**: Create block storage to be consumed by a pod
- **[Shared File System](k8s-filesystem.md)**: Create a file system to be shared across multiple pods
- **[Object](k8s-object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster

## Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

The toolbox also contains the `rookctl` tool as required in the [File System](k8s-filesystem.md) and [Object](k8s-object.md) walkthroughs, or a [simplified walkthrough of block, file and object storage](client.md). In the near future, `rookctl` will not be required for kubernetes scenarios.

### Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./k8s-monitoring.md).

## Teardown

To clean up all the artifacts created by the demo, **first cleanup the resources from the block, file, and object walkthroughs** (unmount volumes, delete volume claims, etc), then run the following:

```bash
kubectl delete -f rook-operator.yaml
kubectl delete -n rook cluster rook
kubectl delete -n rook serviceaccount rook-api
kubectl delete clusterrole rook-api
kubectl delete clusterrolebinding rook-api
kubectl delete thirdpartyresources cluster.rook.io pool.rook.io  # ignore errors if on K8s 1.7+
kubectl delete crd clusters.rook.io pools.rook.io  # ignore errors if on K8s 1.5 and 1.6
kubectl delete secret rook-rook-user
kubectl delete namespace rook
```
If you modified the demo settings, additional cleanup is up to you for devices, host paths, etc.

## Design

With Rook running in the Kubernetes cluster, Kubernetes applications can
mount block devices and filesystems managed by Rook, or can use the S3/Swift API for object storage. The Rook operator
automates configuration of the Ceph storage components and monitors the cluster to ensure the storage remains available
and healthy. There is also a REST API service for configuring the Rook storage and a command line tool called `rookctl`.

![Rook Architecture on Kubernetes](media/kubernetes.png)

The Rook operator is a simple container that has all that is needed to bootstrap
and monitor the storage cluster. The operator will start and monitor ceph monitor pods and a daemonset for the OSDs, which provides basic
RADOS storage as well as a deployment for a RESTful API service. When requested through the api service,
object storage (S3/Swift) is enabled by starting a deployment for RGW, while a shared file system is enabled with a deployment for MDS.

The operator will monitor the storage daemons to ensure the cluster is healthy. Ceph mons will be started or failed over when necessary, and
other adjustments are made as the cluster grows or shrinks.  The operator will also watch for desired state changes
requested by the api service and apply the changes.

The Rook daemons (Mons, OSDs, MGR, RGW, and MDS) are compiled to a single binary `rook`, and included in a minimal container.
The `rook` container includes Ceph daemons and tools to manage and store all data -- there are no changes to the data path.
Rook does not attempt to maintain full fidelity with Ceph. Many of the Ceph concepts like placement groups and crush maps
are hidden so you don't have to worry about them. Instead Rook creates a much simplified UX for admins that is in terms
of physical resources, pools, volumes, filesystems, and buckets.

Rook is implemented in golang. Ceph is implemented in C++ where the data path is highly optimized. We believe
this combination offers the best of both worlds.

See [Design](https://github.com/rook/rook/wiki/Design) wiki for more details.
