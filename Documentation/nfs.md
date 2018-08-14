---
title: Network File System (NFS)
weight: 8
indent: true
---

# Network File System (NFS)

NFS allows remote hosts to mount file systems over a network and interact with those file systems as though they are mounted locally. This enables system administrators to consolidate resources onto centralized servers on the network.

## Prerequisites

1. A Kubernetes cluster is necessary to run the Rook NFS operator. To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).
2. The desired volume to export needs to be attached to the NFS server pod via a [PVC](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims).
Any type of PVC can be attached and exported, such as Host Path, AWS Elastic Block Store, GCP Persistent Disk, CephFS, Ceph RBD, etc.
The limitations of these volumes also apply while they are shared by NFS.
You can read further about the details and limitations of these volumes in the [Kubernetes docs](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).


## Deploy NFS Operator

First deploy the Rook NFS operator using the following commands:

```console
cd cluster/examples/kubernetes/nfs
kubectl create -f operator.yaml
```

You can check if the operator is up and running with:

```console
kubectl -n rook-nfs-system get pod

NAME                                READY     STATUS    RESTARTS   AGE
rook-nfs-operator-b8d6d955d-cdq2v   1/1       Running   0          1m
```

## Create and Initialize NFS Server

Now that the operator is running, we can create an instance of a NFS server by creating an instance of the `nfsservers.nfs.rook.io` resource.
The various fields and options of the NFS server resource can be used to configure the server and its volumes to export.
Full details of the available configuration options can be found in the [NFS CRD documentation](nfs-crd.md).

This guide has 2 main examples that demonstrate exporting volumes with a NFS server:

1. [Default StorageClass example](#default-storageclass-example)
1. [Rook Ceph volume example](#rook-ceph-volume-example)

## Default StorageClass example

This first example will walk through creating a NFS server instance that exports storage that is backed by the default `StorageClass` for the environment you happen to be running in.
In some environments, this could be a host path, in others it could be a cloud provider virtual disk.
Either way, this example requires a default `StorageClass` to exist.

Start by saving the below NFS CRD instance definition to a file called `nfs.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name:  rook-nfs
---
# A default StorageClass must be present
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-default-claim
  namespace: rook-nfs
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      accessMode: ReadWrite
      squash: "none"
    # A Persistent Volume Claim must be created before creating NFS CRD instance.
    persistentVolumeClaim:
      claimName: nfs-default-claim
```

With the `nfs.yaml` file saved, now create the NFS server as shown:

```console
kubectl create -f nfs.yaml
```

We can verify that a Kubernetes object has been created that represents our new NFS server and its export with the command below.

```console
kubectl -n rook-nfs get nfsservers.nfs.rook.io

NAME       AGE
rook-nfs   1m
```

Verify that the NFS server pod is up and running:

```console
kubectl -n rook-nfs get pod -l app=rook-nfs

NAME         READY     STATUS    RESTARTS   AGE
rook-nfs-0   1/1       Running   0          2m
```

If the NFS server pod is in the `Running` state, then we have successfully created an exported NFS share that clients can start to access over the network.

### Accessing the Export

To access the export from another pod, you must first manually create a `PersistentVolume` with the connection information.
This experience is a bit cumbersome, but will be improved in the future with dynamic provisioning support.
First, find the current IP address of your NFS server pod using the following command:

```console
kubectl -n rook-nfs get service -l app=rook-nfs -o jsonpath='{.items[0].spec.clusterIP}'
```

Copy this IP address and insert it into the following content in place of the `<Cluster IP>` placeholder.
Then also replace the `/<Claim name>` value with the `claimName` that was configured in the `nfs.yaml`.
In this case, it will be `nfs-default-claim` and it will look like `path: "/nfs-default-claim"`.
Finally, save the updated content to a file called `pv.yaml`.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: rook-nfs-pv
  namespace: rook-nfs
spec:
  capacity:
    storage: 1Mi
  accessModes:
    - ReadWriteMany
  nfs:
    # FIXME: replace <Cluster IP> with the current IP of the NFS server pod.
    # Run the below command to get the current IP address:
    # kubectl -n rook-nfs get service -l app=rook-nfs -o jsonpath='{.items[0].spec.clusterIP}'
    server: <Cluster IP>
    path: "/<Claim name>"
```

With the `pv.yaml` file saved, we can create the PV object:

```console
kubectl create -f pv.yaml
```

### Consuming the Export

Now we can consume the PV that we just created by creating an example web server app that uses a `PersistentVolumeClaim` to claim the exported volume.
There are 2 pods that comprise this example:

1. A web server pod that will read and display the contents of the NFS share
1. A writer pod that will write random data to the NFS share so the website will continually update

First, save the PVC definition in a file called `pvc.yaml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rook-nfs-pv-claim
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: ""
  resources:
    requests:
      storage: 1Mi
```

Then create the PVC:

```console
kubectl create -f pvc.yaml
```

Start both the busybox pod (writer) and the web server from the `cluster/examples/kubernetes/nfs` folder:

```console
kubectl create -f busybox-rc.yaml
kubectl create -f web-rc.yaml
```

Let's confirm that the expected busybox writer pods and web server pods are **all** up and in the `Running` state:

```console
kubectl get pod -l app=nfs-demo
```

In order to be able to reach the web server over the network, let's create a service for it:

```console
kubectl create -f web-service.yaml
```

We can then use the busybox writer pod we launched before to check that nginx is serving the data appropriately.
In the below 1-liner command, we use `kubectl exec` to run a command in the busybox writer pod that uses `wget` to retrieve the web page that the web server pod is hosting.  As the busybox writer pod continues to write a new timestamp, we should see the returned output also update every ~10 seconds or so.

```console
> echo; kubectl exec $(kubectl get pod -l name=nfs-busybox -o jsonpath='{.items[0].metadata.name}') -- wget -qO- http://$(kubectl get services nfs-web -o jsonpath='{.spec.clusterIP}'); echo

Thu Oct 22 19:28:55 UTC 2015
nfs-busybox-w3s4t

```

## Rook Ceph volume example

In this alternative example, we will use a different underlying volume as an export for the NFS server.
These steps will walk us through exporting a Ceph RBD block volume so that clients can access it across the network.

First, you have to [follow these instructions](ceph-quickstart.md) to deploy a sample Rook Ceph cluster that can be attached to the NFS server pod for sharing.
After the Rook Ceph cluster is up and running, we can create proceed with creating the NFS server.

Save this PVC and NFS CRD instance as `nfs-ceph.yaml`:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name:  rook-nfs
---
# A rook ceph cluster must be running
# Create a rook ceph cluster using examples in rook/cluster/examples/kubernetes/ceph
# Refer to https://rook.io/docs/rook/master/ceph-quickstart.html for a quick rook cluster setup
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-ceph-claim
  namespace: rook-nfs
spec:
  storageClassName: rook-ceph-block
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 2Gi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      accessMode: ReadWrite
      squash: "none"
    # A Persistent Volume Claim must be created before creating NFS CRD instance.
    # Create a Ceph cluster for using this example
    # Create a ceph PVC after creating the rook ceph cluster using ceph-pvc.yaml
    persistentVolumeClaim:
      claimName: nfs-ceph-claim
```

Create the NFS server instance that you saved in `nfs-ceph.yaml`:

```console
kubectl create -f nfs-ceph.yaml
```

After the NFS server pod is running, follow the same instructions from the previous example to access and consume the NFS share, with the following exception:

* Replace the `/<Claim name>` value in the PV definition in the [Accessing the Export](nfs.md#accessing-the-export) section with the `claimName` that was configured in the `nfs-ceph.yaml`. In this case it will be `nfs-ceph-claim` and will look like `path: "/nfs-ceph-claim"`.

After that, follow the rest of the instructions in the [Accessing the Export](nfs.md#accessing-the-export) section and then the [Consuming the Export](nfs.md#consuming-the-export) section to consume the NFS volume.

## Teardown

To clean up all resources associated with this walk-through, you can run the commands below.

```console
kubectl delete -f web-service.yaml
kubectl delete -f web-rc.yaml
kubectl delete -f busybox-rc.yaml
kubectl delete -f pvc.yaml
kubectl delete -f pv.yaml
kubectl delete -f nfs.yaml
kubectl delete -f nfs-ceph.yaml
kubectl delete -f operator.yaml
```

## Troubleshooting

If the NFS server pod does not come up, the first step would be to examine the NFS operator's logs:

```console
kubectl -n rook-nfs-system logs -l app=rook-nfs-operator
```