
# Rook on Kubernetes
- [Quickstart](#quickstart)
- [Design](#design)

## Quickstart
This example shows how to build a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

### Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these quick instructions](k8s-pre-reqs.md), including:
- The `kubelet` requires access to `modprobe` 
- If RBAC is enabled, the operator must be given privileges

Note that we are striving for even more smooth integration with Kubernetes in the future such that `Rook` will work out of the box with any Kubernetes cluster.

### Deploy Rook

With your Kubernetes cluster running, Rook can be setup and deployed by simply creating the [rook-operator](/demo/kubernetes/rook-operator.yaml) deployment and creating a [rook cluster](/demo/kubernetes/rook-cluster.yaml).

```bash
cd demo/kubernetes
kubectl create -f rook-operator.yaml

# This will start the rook-operator pod.  Verify that it is in the `Running` state before proceeding:
kubectl get pod | grep rook-operator
```

Now that the rook-operator pod is running, we can create the Rook cluster. See the documentation on [configuring the cluster](cluster-tpr.md).
```bash
kubectl create -f rook-cluster.yaml
```

Use `kubectl` to list pods in the rook namespace. You should be able to see the following pods once they are all running: 

```bash
$ kubectl -n rook get pod
NAME                        READY     STATUS    RESTARTS   AGE
mon0                        1/1       Running   0          1m
mon1                        1/1       Running   0          1m
mon2                        1/1       Running   0          1m
osd-3n85p                   1/1       Running   0          1m
osd-6jmph                   1/1       Running   0          1m
rook-api-1709486253-gvdnc   1/1       Running   0          1m
```

## Storage
For a walkthrough of the three types of storage exposed by Rook, see the guides for:
- **[Block](k8s-block.md)**: Create block storage to be consumed by a pod
- **[Shared File System](k8s-filesystem.md)**: Create a file system to be shared across multiple pods
- **[Object](k8s-object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster

## Tools

### Rook Client
You also have the option to use the `rook` client tool directly by running it in a pod that can be started in the cluster with:
```bash
kubectl create -f rook-client/rook-client.yml

# Starting the rook-client pod will take a bit of time to download the container, so check when it's in the Running state
kubectl -n rook get pod rook-client

# Connect to the rook-client pod 
kubectl -n rook exec rook-client -it bash

# Verify the rook client can talk to the cluster:
rook node ls
```

At this point, you can use the `rook` tool along with some [simple steps to create and manage block, file and object storage](client.md).

### Advanced Configuration and Troubleshooting
We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](toolbox.md) for setup and usage information.

### Monitoring
Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./k8s-monitoring.md).

## Teardown
To clean up all the artifacts created by the demo, *first cleanup the resources from the block, file, and object walkthroughs* (unmount volumes, delete volume claims, etc), then run the following:
```bash
kubectl delete deployment rook-operator
kubectl delete -n rook rookcluster rook
kubectl delete thirdpartyresources rookcluster.rook.io rookpool.rook.io
kubectl delete secret rook-rbd-user
kubectl delete namespace rook
```
If you modified the demo settings, additional cleanup is up to you for devices, host paths, etc.

## Design

With Rook running in the Kubernetes cluster, Kubernetes applications can
mount block devices and filesystems managed by Rook, or can use the S3/Swift API for object storage. The Rook operator 
automates configuration of the Ceph storage components and monitors the cluster to ensure the storage remains available
and healthy. There is also a REST API service for configuring the Rook storage and a command line tool called `rook`.

![Rook Architecture on Kubernetes](media/kubernetes.png)

The Rook operator is a simple container that has all that is needed to bootstrap
and monitor the storage cluster. The operator will start and monitor ceph monitor pods and a daemonset for the OSDs, which provides basic
RADOS storage as well as a deployment for a RESTful API service. When requested through the api service,
object storage (S3/Swift) is enabled by starting a deployment for RGW, while a shared file system is enabled with a deployment for MDS.

The operator will monitor the storage daemons to ensure the cluster is healthy. Ceph mons will be started or failed over when necessary, and
other adjustments are made as the cluster grows or shrinks.  The operator will also watch for desired state changes
requested by the api service and apply the changes.

The Rook daemons (Mons, OSDs, RGW, and MDS) are compiled to a single binary `rookd`, and included in a minimal container.
`rookd` uses an embedded version of Ceph for storing all data -- there are no changes to the data path. 
Rook does not attempt to maintain full fidelity with Ceph. Many of the Ceph concepts like placement groups and crush maps 
are hidden so you don't have to worry about them. Instead Rook creates a much simplified UX for admins that is in terms 
of physical resources, pools, volumes, filesystems, and buckets.

Rook is implemented in golang. Ceph is implemented in C++ where the data path is highly optimized. We believe
this combination offers the best of both worlds.

See [Design](https://github.com/rook/rook/wiki/Design) wiki for more details.