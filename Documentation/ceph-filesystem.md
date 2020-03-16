---
title: Shared Filesystem
weight: 2300
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Shared Filesystem

A shared filesystem can be mounted with read/write permission from multiple pods. This may be useful for applications which can be clustered using a shared filesystem.

This example runs a shared filesystem for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](ceph-quickstart.md)

### Multiple Filesystems Not Supported

By default only one shared filesystem can be created with Rook. Multiple filesystem support in Ceph is still considered experimental and can be enabled with the environment variable `ROOK_ALLOW_MULTIPLE_FILESYSTEMS` defined in `operator.yaml`.

Please refer to [cephfs experimental features](http://docs.ceph.com/docs/master/cephfs/experimental-features/#multiple-filesystems-within-a-ceph-cluster) page for more information.

## Create the Filesystem

Create the filesystem by specifying the desired settings for the metadata pool, data pools, and metadata server in the `CephFilesystem` CRD. In this example we create the metadata pool with replication of three and a single data pool with replication of three. For more options, see the documentation on [creating shared filesystems](ceph-filesystem-crd.md).

Save this shared filesystem definition as `filesystem.yaml`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataPool:
    replicated:
      size: 3
  dataPools:
    - replicated:
        size: 3
  preservePoolsOnDelete: true
  metadataServer:
    activeCount: 1
    activeStandby: true
```

The Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.

```console
## Create the filesystem
$ kubectl create -f filesystem.yaml
[...]
## To confirm the filesystem is configured, wait for the mds pods to start
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mds
NAME                                      READY     STATUS    RESTARTS   AGE
rook-ceph-mds-myfs-7d59fdfcf4-h8kw9       1/1       Running   0          12s
rook-ceph-mds-myfs-7d59fdfcf4-kgkjp       1/1       Running   0          12s
```

To see detailed status of the filesystem, start and connect to the [Rook toolbox](ceph-toolbox.md). A new line will be shown with `ceph status` for the `mds` service. In this example, there is one active instance of MDS which is up, with one MDS instance in `standby-replay` mode in case of failover.

```console
$ ceph status
  ...
  services:
    mds: myfs-1/1/1 up {[myfs:0]=mzw58b=up:active}, 1 up:standby-replay
```

## Provision Storage

Before Rook can start provisioning storage, a StorageClass needs to be created based on the filesystem. This is needed for Kubernetes to interoperate
with the CSI driver to create persistent volumes.

> **NOTE**: This example uses the CSI driver, which is the preferred driver going forward for K8s 1.13 and newer. Examples are found in the [CSI CephFS](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/cephfs) directory. For an example of a volume using the flex driver (required for K8s 1.12 and earlier), see the [Flex Driver](#flex-driver) section below.

Save this storage class definition as `storageclass.yaml`:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-cephfs
# Change "rook-ceph" provisioner prefix to match the operator namespace if needed
provisioner: rook-ceph.cephfs.csi.ceph.com
parameters:
  # clusterID is the namespace where operator is deployed.
  clusterID: rook-ceph

  # CephFS filesystem name into which the volume shall be created
  fsName: myfs

  # Ceph pool into which the volume shall be created
  # Required for provisionVolume: "true"
  pool: myfs-data0

  # Root path of an existing CephFS volume
  # Required for provisionVolume: "false"
  # rootPath: /absolute/path

  # The secrets contain Ceph admin credentials. These are generated automatically by the operator
  # in the same namespace as the cluster.
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph

reclaimPolicy: Delete
```

If you've deployed the Rook operator in a namespace other than "rook-ceph"
as is common change the prefix in the provisioner to match the namespace
you used. For example, if the Rook operator is running in "rook-op" the
provisioner value should be "rook-op.rbd.csi.ceph.com".

Create the storage class.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/cephfs/storageclass.yaml
```

## Quotas

> **IMPORTANT**: The CephFS CSI driver uses quotas to enforce the PVC size requested.
Only newer kernels support CephFS quotas (kernel version of at least 4.17).
If you require quotas to be enforced and the kernel driver does not support it, you can disable the kernel driver
and use the FUSE client. This can be done by setting `CSI_FORCE_CEPHFS_KERNEL_CLIENT: false`
in the operator deployment (`operator.yaml`). However, it is important to know that when
the FUSE client is enabled, there is an issue that during upgrade the application pods will be
disconnected from the mount and will need to be restarted. See the [upgrade guide](ceph-upgrade.md)
for more details.

## Consume the Shared Filesystem: K8s Registry Sample

As an example, we will start the kube-registry pod with the shared filesystem as the backing store.
Save the following spec as `kube-registry.yaml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cephfs-pvc
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: rook-cephfs
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-registry
  namespace: kube-system
  labels:
    k8s-app: kube-registry
    kubernetes.io/cluster-service: "true"
spec:
  replicas: 3
  selector:
    matchLabels:
      k8s-app: kube-registry
  template:
    metadata:
      labels:
        k8s-app: kube-registry
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: registry
        image: registry:2
        imagePullPolicy: Always
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
        env:
        # Configuration reference: https://docs.docker.com/registry/configuration/
        - name: REGISTRY_HTTP_ADDR
          value: :5000
        - name: REGISTRY_HTTP_SECRET
          value: "Ple4seCh4ngeThisN0tAVerySecretV4lue"
        - name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
          value: /var/lib/registry
        volumeMounts:
        - name: image-store
          mountPath: /var/lib/registry
        ports:
        - containerPort: 5000
          name: registry
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /
            port: registry
        readinessProbe:
          httpGet:
            path: /
            port: registry
      volumes:
      - name: image-store
        persistentVolumeClaim:
          claimName: cephfs-pvc
          readOnly: false
```

Create the Kube registry deployment:

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/cephfs/kube-registry.yaml
```

You now have a docker registry which is HA with persistent storage.

### Kernel Version Requirement

If the Rook cluster has more than one filesystem and the application pod is scheduled to a node with kernel version older than 4.7, inconsistent results may arise since kernels older than 4.7 do not support specifying filesystem namespaces.

## Consume the Shared Filesystem: Toolbox

Once you have pushed an image to the registry (see the [instructions](https://github.com/kubernetes/kubernetes/tree/release-1.9/cluster/addons/registry) to expose and use the kube-registry), verify that kube-registry is using the filesystem that was configured above by mounting the shared filesystem in the toolbox pod. See the [Direct Filesystem](direct-tools.md#shared-filesystem-tools) topic for more details.

## Teardown

To clean up all the artifacts created by the filesystem demo:

```console
kubectl delete -f kube-registry.yaml
```

To delete the filesystem components and backing data, delete the Filesystem CRD.

> **WARNING: Data will be deleted if preservePoolsOnDelete=false**.

```console
kubectl -n rook-ceph delete cephfilesystem myfs
```

Note: If the "preservePoolsOnDelete" filesystem attribute is set to true, the above command won't delete the pools. Creating again the filesystem with the same CRD will reuse again the previous pools.

## Flex Driver

To create a volume based on the flex driver instead of the CSI driver, see the [kube-registry.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/flex/kube-registry.yaml) example manifest or refer to the complete flow in the Rook v1.0 [Shared Filesystem](https://rook.io/docs/rook/v1.0/ceph-filesystem.html) documentation.

### Advanced Example: Erasure Coded Filesystem

The Ceph filesystem example can be found here: [Ceph Shared Filesystem - Samples - Erasure Coded](ceph-filesystem-crd.md#erasure-coded).
