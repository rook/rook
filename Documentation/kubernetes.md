
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

```
cd demo/kubernetes
kubectl create -f rook-operator.yaml
```

This will start the rook-operator pod.  Verify that it is in the `Running` state before proceeding:
```
kubectl get pod | grep rook-operator | awk '{print $3}'
```

Now that the rook-operator pod is in the `Running` state, we can create the Rook cluster. See the documentation on [configuring the cluster](cluster-tpr.md).
```
kubectl create -f rook-cluster.yaml
```

Use `kubectl` to list pods in the rook namespace. You should be able to see the following once they are all running: 

```
$ kubectl -n rook get pod
NAME                        READY     STATUS    RESTARTS   AGE
mon0                        1/1       Running   0          1m
mon1                        1/1       Running   0          1m
mon2                        1/1       Running   0          1m
osd-3n85p                   1/1       Running   0          1m
osd-6jmph                   1/1       Running   0          1m
rook-api-1709486253-gvdnc   1/1       Running   0          1m
```

### Provision Storage
Before Rook can start provisioning storage, a StorageClass and its storage pool need to be created. This is needed for Kubernetes to interoperate with Rook for provisioning persistent volumes.  

First create the storage pool. See the documentation on [creating storage pools](pool-tpr.md).
```
kubectl create -f rook-pool.yaml
```

Rook already creates a default admin and rbd user, whose secrets are already specified in the sample [rook-storageclass.yaml](/demo/kubernetes/rook-storageclass.yaml). Now we just need to specify the Ceph monitor endpoints (requires `jq`):

```
export MONS=$(kubectl -n rook get pod mon0 mon1 mon2 -o json|jq ".items[].status.podIP"|tr -d "\""|sed -e 's/$/:6790/'|paste -s -d, -)
sed 's#INSERT_HERE#'$MONS'#' rook-storageclass.yaml | kubectl create -f -
``` 
**NOTE:** In the v0.4 release we plan to expose monitors via DNS/service names instead of IP address (see [#355](https://github.com/rook/rook/issues/355)), which will streamline the experience and remove the need for this step.

### Consume the storage

Now that Rook is running and integrated with Kubernetes, we can create a sample app to consume the block storage provisioned by Rook. We will create the classic wordpress and mysql apps.
Both these apps will make use of block volumes provisioned by Rook.

Start mysql and wordpress from the `demo/kubernetes` folder:

```
kubectl create -f mysql.yaml
kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Once the wordpress and mysql pods are in the `Running` state, get the cluster IP of the wordpress app and enter it in your brower:

```
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.  

**NOTE:** When running in a vagrant environment, there will be no external IP address to reach wordpress with.  You will only be able to reach wordpress via the `CLUSTER-IP` from inside the Kubernetes cluster.

### Rook Client
You also have the option to use the `rook` client tool directly by running it in a pod that can be started in the cluster with:
```
kubectl create -f rook-client/rook-client.yml
```  

Starting the rook-client pod will take a bit of time to download the container, so you can check to see when it's ready with (it should be in the `Running` state):
```
kubectl -n rook get pod rook-client
```

Connect to the rook-client pod and verify the `rook` client can talk to the cluster:
```
kubectl -n rook exec rook-client -it bash
rook node ls
```

At this point, you can use the `rook` tool along with some [simple steps to create and manage block, file and object storage](client.md).

### Teardown
To clean up all the artifacts created by the demo, run the following:
```
kubectl delete -f wordpress.yaml
kubectl delete -f mysql.yaml
kubectl delete deployment rook-operator
kubectl delete thirdpartyresources rookcluster.rook.io rookpool.rook.io
kubectl delete storageclass rook-block
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

The Rook operator is a simple container containing the `rook-operator` binary that has all that is needed to bootstrap
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