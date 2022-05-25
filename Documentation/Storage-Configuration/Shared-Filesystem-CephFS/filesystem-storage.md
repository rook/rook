---
title: Filesystem Storage Overview
---

A filesystem storage (also named shared filesystem) can be mounted with read/write permission from multiple pods. This may be useful for applications which can be clustered using a shared filesystem.

This example runs a shared filesystem for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/release-1.9/cluster/addons/registry).

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [quickstart guide](../../Getting-Started/quickstart.md)

### Multiple Filesystems Support

Multiple filesystems are supported as of the Ceph Pacific release.

## Create the Filesystem

Create the filesystem by specifying the desired settings for the metadata pool, data pools, and metadata server in the `CephFilesystem` CRD. In this example we create the metadata pool with replication of three and a single data pool with replication of three. For more options, see the documentation on [creating shared filesystems](../../CRDs/Shared-Filesystem/ceph-filesystem-crd.md).

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
    - name: replicated
      replicated:
        size: 3
  preserveFilesystemOnDelete: true
  metadataServer:
    activeCount: 1
    activeStandby: true
```

The Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.

```console
# Create the filesystem
kubectl create -f filesystem.yaml
[...]
```

To confirm the filesystem is configured, wait for the mds pods to start:

```console
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mds
NAME                                      READY     STATUS    RESTARTS   AGE
rook-ceph-mds-myfs-7d59fdfcf4-h8kw9       1/1       Running   0          12s
rook-ceph-mds-myfs-7d59fdfcf4-kgkjp       1/1       Running   0          12s
```

To see detailed status of the filesystem, start and connect to the [Rook toolbox](../../Troubleshooting/ceph-toolbox.md). A new line will be shown with `ceph status` for the `mds` service. In this example, there is one active instance of MDS which is up, with one MDS instance in `standby-replay` mode in case of failover.

```console
$ ceph status
[...]
  services:
    mds: myfs-1/1/1 up {[myfs:0]=mzw58b=up:active}, 1 up:standby-replay
```

## Provision Storage

Before Rook can start provisioning storage, a StorageClass needs to be created based on the filesystem. This is needed for Kubernetes to interoperate
with the CSI driver to create persistent volumes.

Save this storage class definition as `storageclass.yaml`:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-cephfs
# Change "rook-ceph" provisioner prefix to match the operator namespace if needed
provisioner: rook-ceph.cephfs.csi.ceph.com
parameters:
  # clusterID is the namespace where the rook cluster is running
  # If you change this namespace, also change the namespace below where the secret namespaces are defined
  clusterID: rook-ceph

  # CephFS filesystem name into which the volume shall be created
  fsName: myfs

  # Ceph pool into which the volume shall be created
  # Required for provisionVolume: "true"
  pool: myfs-replicated

  # The secrets contain Ceph admin credentials. These are generated automatically by the operator
  # in the same namespace as the cluster.
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
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
kubectl create -f deploy/examples/csi/cephfs/storageclass.yaml
```

## Quotas

!!! attention
    The CephFS CSI driver uses quotas to enforce the PVC size requested.

Only newer kernels support CephFS quotas (kernel version of at least 4.17).
If you require quotas to be enforced and the kernel driver does not support it, you can disable the kernel driver
and use the FUSE client. This can be done by setting `CSI_FORCE_CEPHFS_KERNEL_CLIENT: false`
in the operator deployment (`operator.yaml`). However, it is important to know that when
the FUSE client is enabled, there is an issue that during upgrade the application pods will be
disconnected from the mount and will need to be restarted. See the [upgrade guide](../../Upgrade/rook-upgrade.md)
for more details.

## Consume the Shared Filesystem: K8s Registry Sample

As an example, we will start the kube-registry pod with the shared filesystem as the backing store.
Save the following spec as `kube-registry.yaml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cephfs-pvc
  namespace: kube-system
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
kubectl create -f deploy/examples/csi/cephfs/kube-registry.yaml
```

You now have a docker registry which is HA with persistent storage.

### Kernel Version Requirement

If the Rook cluster has more than one filesystem and the application pod is scheduled to a node with kernel version older than 4.7, inconsistent results may arise since kernels older than 4.7 do not support specifying filesystem namespaces.

## Consume the Shared Filesystem: Toolbox

Once you have pushed an image to the registry (see the [instructions](https://github.com/kubernetes/kubernetes/tree/release-1.9/cluster/addons/registry) to expose and use the kube-registry), verify that kube-registry is using the filesystem that was configured above by mounting the shared filesystem in the toolbox pod. See the [Direct Filesystem](../../Troubleshooting/direct-tools.md#shared-filesystem-tools) topic for more details.

## Teardown

To clean up all the artifacts created by the filesystem demo:

```console
kubectl delete -f kube-registry.yaml
```

To delete the filesystem components and backing data, delete the Filesystem CRD.

!!! warning
    Data will be deleted if preserveFilesystemOnDelete=false**.

```console
kubectl -n rook-ceph delete cephfilesystem myfs
```

Note: If the "preserveFilesystemOnDelete" filesystem attribute is set to true, the above command won't delete the filesystem. Recreating the same CRD will reuse the existing filesystem.

### Advanced Example: Erasure Coded Filesystem

The Ceph filesystem example can be found here: [Ceph Shared Filesystem - Samples - Erasure Coded](../../CRDs/Shared-Filesystem/ceph-filesystem-crd.md#erasure-coded).
