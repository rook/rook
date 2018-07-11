---
title: Shared File System
weight: 26
indent: true
---

# Shared File System

A shared file system can be mounted with read/write permission from multiple pods. This may be useful for applications which can be clustered using a shared filesystem.

This example runs a shared file system for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

### Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](quickstart.md)

### Multiple File Systems Not Supported

By default only one shared file system can be created with Rook. Multiple file system support in Ceph is still considered experimental and can be enabled with the environment variable `ROOK_ALLOW_MULTIPLE_FILESYSTEMS` defined in `rook-operator.yaml`.

Please refer to [cephfs experimental features](http://docs.ceph.com/docs/master/cephfs/experimental-features/#multiple-filesystems-within-a-ceph-cluster) page for more information.

## Create the File System

Create the file system by specifying the desired settings for the metadata pool, data pools, and metadata server in the `Filesystem` CRD. In this example we create the metadata pool with replication of three and a single data pool with erasure coding. For more options, see the documentation on [creating shared file systems](ceph-filesystem-crd.md).

Save this shared file system definition as `filesystem.yaml`:

```yaml
apiVersion: ceph.rook.io/v1beta1
kind: Filesystem
metadata:
  name: myfs
  namespace: rook-ceph
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

The Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.
```bash
# Create the file system
$ kubectl create -f filesystem.yaml

# To confirm the file system is configured, wait for the mds pods to start
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mds
NAME                                      READY     STATUS    RESTARTS   AGE
rook-ceph-mds-myfs-7d59fdfcf4-h8kw9       1/1       Running   0          12s
rook-ceph-mds-myfs-7d59fdfcf4-kgkjp       1/1       Running   0          12s
```

To see detailed status of the file system, start and connect to the [Rook toolbox](toolbox.md). A new line will be shown with `ceph status` for the `mds` service. In this example, there is one active instance of MDS which is up, with one MDS instance in `standby-replay` mode in case of failover.

```bash
$ ceph status
  ...
  services:
    mds: myfs-1/1/1 up {[myfs:0]=mzw58b=up:active}, 1 up:standby-replay
```

## Consume the Shared File System: K8s Registry Sample

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
          driver: ceph.rook.io/rook
          fsType: ceph
          options:
            fsName: myfs # name of the filesystem specified in the filesystem CRD.
            clusterNamespace: rook-ceph # namespace where the Rook cluster is deployed
            # by default the path is /, but you can override and mount a specific path of the filesystem by using the path attribute
            # the path must exist on the filesystem, otherwise mounting the filesystem at that path will fail
            # path: /some/path/inside/cephfs
```

After creating it with `kubectl create -f kube-registry.yaml`, you now have a docker registry which is HA with persistent storage.

#### Kernel Version Requirement
If the Rook cluster has more than one filesystem and the application pod is scheduled to a node with kernel version older than 4.7, inconsistent results may arise since kernels older than 4.7 do not support specifying filesystem namespaces.

## Consume the Shared File System: Toolbox

Once you have pushed an image to the registry (see the [instructions](https://github.com/kubernetes/kubernetes/tree/release-1.9/cluster/addons/registry) to expose and use the kube-registry), verify that kube-registry is using the filesystem that was configured above by mounting the shared file system in the toolbox pod. See the [Direct Filesystem](direct-tools.md#shared-filesystem-tools) topic for more details.


## Teardown
To clean up all the artifacts created by the file system demo:
```bash
kubectl delete -f kube-registry.yaml
```

To delete the filesystem components and backing data, delete the Filesystem CRD. **Warning: Data will be deleted**
```
kubectl -n rook-ceph delete Filesystem myfs
```
