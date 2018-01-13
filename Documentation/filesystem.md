---
title: Shared File System
weight: 26
indent: true
---

# Shared File System

A shared file system can be mounted read-write from multiple pods. This may be useful for applications which can be clustered using a shared filesystem.

This example runs a shared file system for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](quickstart.md)

## Create the File System

Create the file system by specifying the desired settings for the metadata pool, data pools, and metadata server in the `Filesystem` CRD. In this example we create the metadata pool with replication of three and a single data pool with erasure coding. For more options, see the documentation on [creating shared file systems](filesystem-crd.md).

Save this shared file system definition as `rook-filesystem.yaml`:

```yaml
apiVersion: rook.io/v1alpha1
kind: Filesystem
metadata:
  name: myfs
  namespace: rook
spec:
  metadataPool:
    replicated:
      size: 3
  dataPools:
    - erasureCoded:
       dataChunks: 2
       codingChunks: 1
  metadataServer:
    activeCount: 1
    activeStandby: true
```

Now let's create the file system. The Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.
```bash
# Create the file system
kubectl create -f rook-filesystem.yaml

# To confirm the file system is configured, wait for the mds pods to start
kubectl -n rook get pod -l app=rook-ceph-mds
```

To see detailed status of the file system, start and connect to the [Rook toolbox](toolbox.md). A new line will be shown with `ceph status` for the `mds` service. In this example, there is one active instance of MDS which is up, with one MDS instance in `standby-replay` mode in case of failover.

```bash
$ ceph status                                                                                                                                              
  ...
  services:
    mds: myfs-1/1/1 up {[myfs:0]=mzw58b=up:active}, 1 up:standby-replay
```

## Consume the file system

As an example, we will start the kube-registry pod with the shared file system as the backing store. 
Save the following spec as `kube-registry.yaml`:

```yaml
apiVersion: v1
kind: ReplicationController
metadata:
  name: kube-registry-v0
  namespace: kube-system
  labels:
    k8s-app: kube-registry
    version: v0
    kubernetes.io/cluster-service: "true"
spec:
  replicas: 3
  selector:
    k8s-app: kube-registry
    version: v0
  template:
    metadata:
      labels:
        k8s-app: kube-registry
        version: v0
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: registry
        image: registry:2
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
        env:
        - name: REGISTRY_HTTP_ADDR
          value: :5000
        - name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
          value: /var/lib/registry
        volumeMounts:
        - name: image-store
          mountPath: /var/lib/registry
        ports:
        - containerPort: 5000
          name: registry
          protocol: TCP
      volumes:
      - name: image-store
        flexVolume:
          driver: rook.io/rook
          fsType: ceph
          options:
            fsName: myfs # name of the filesystem specified in the filesystem CRD.
            clusterName: rook # namespace where the Rook cluster is deployed
            # by default the path is /, but you can override and mount a specific path of the filesystem by using the path attribute
            # path: /some/path/inside/cephfs
```

You now have a docker registry which is HA with persistent storage.

#### Kernel Version Requirement
If the Rook cluster has more than one filesystem and the application pod is scheduled to a node with kernel version older than 4.7, inconsistent results may arise since kernels older than 4.7 do not support specifying filesystem namespaces.

## Test the storage

Once you have pushed an image to the registry (see the [instructions](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry) to expose and use the kube-registry), verify that kube-registry is using the filesystem that was configured above by mounting the shared file system in the toolbox pod.

Start and connect to the [Rook toolbox](toolbox.md).

```bash
# Mount the same filesystem that the kube-registry is using
mkdir /tmp/registry
rookctl filesystem mount --name myfs --path /tmp/registry

# If you have pushed images to the registry you will see a directory called docker
ls /tmp/registry

# Cleanup the filesystem mount
rookctl filesystem unmount --path /tmp/registry
rmdir /tmp/registry
```

## Teardown
To clean up all the artifacts created by the file system demo:
```bash
kubectl -n kube-system delete secret rook-admin
kubectl delete -f kube-registry.yaml
```
