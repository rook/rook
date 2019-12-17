---
title: Block Storage
weight: 2100
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Block Storage

Block storage allows a single pod to mount storage. This guide shows how to create a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](ceph-quickstart.md).

## Provision Storage

Before Rook can provision storage, a [`StorageClass`](https://kubernetes.io/docs/concepts/storage/storage-classes) and [`CephBlockPool`](ceph-pool-crd.md) need to be created. This will allow Kubernetes to interoperate with Rook when provisioning persistent volumes.

> **NOTE**: This sample requires *at least 1 OSD per node*, with each OSD located on *3 different nodes*.

Each OSD must be located on a different node, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `host` and the `replicated.size` is set to `3`.

> **NOTE**: This example uses the CSI driver, which is the preferred driver going forward for K8s 1.13 and newer. Examples are found in the [CSI RBD](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/csi/rbd) directory. For an example of a storage class using the flex driver (required for K8s 1.12 or earlier), see the [Flex Driver](#flex-driver) section below, which has examples in the [flex](https://github.com/rook/rook/tree/{{ branchName }}/cluster/examples/kubernetes/ceph/flex) directory.

Save this `StorageClass` definition as `storageclass.yaml`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-ceph-block
# Change "rook-ceph" provisioner prefix to match the operator namespace if needed
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
    # clusterID is the namespace where the rook cluster is running
    clusterID: rook-ceph
    # Ceph pool into which the RBD image shall be created
    pool: replicapool

    # RBD image format. Defaults to "2".
    imageFormat: "2"

    # RBD image features. Available for imageFormat: "2". CSI RBD currently supports only `layering` feature.
    imageFeatures: layering

    # The secrets contain Ceph admin credentials.
    csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
    csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
    csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
    csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph

    # Specify the filesystem type of the volume. If not specified, csi-provisioner
    # will set default as `ext4`.
    csi.storage.k8s.io/fstype: xfs

# Delete the rbd volume when a PVC is deleted
reclaimPolicy: Delete
```

If you've deployed the Rook operator in a namespace other than "rook-ceph"
as is common change the prefix in the provisioner to match the namespace
you used. For example, if the Rook operator is running in "rook-op" the
provisioner value should be "rook-op.rbd.csi.ceph.com".

Create the storage class.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/rbd/storageclass.yaml
```

> **NOTE**: As [specified by Kubernetes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#retain), when using the `Retain` reclaim policy, any Ceph RBD image that is backed by a `PersistentVolume` will continue to exist even after the `PersistentVolume` has been deleted. These Ceph RBD images will need to be cleaned up manually using `rbd rm`.

## Consume the storage: Wordpress sample

We create a sample app to consume the block storage provisioned by Rook with the classic wordpress and mysql apps.
Both of these apps will make use of block volumes provisioned by Rook.

Start mysql and wordpress from the `cluster/examples/kubernetes` folder:

```console
kubectl create -f mysql.yaml
kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```console
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Once the wordpress and mysql pods are in the `Running` state, get the cluster IP of the wordpress app and enter it in your browser:

```console
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.

If you are using Minikube, the Wordpress URL can be retrieved with this one-line command:

```console
echo http://$(minikube ip):$(kubectl get service wordpress -o jsonpath='{.spec.ports[0].nodePort}')
```

> **NOTE**: When running in a vagrant environment, there will be no external IP address to reach wordpress with.  You will only be able to reach wordpress via the `CLUSTER-IP` from inside the Kubernetes cluster.

## Consume the storage: Toolbox

With the pool that was created above, we can also create a block image and mount it directly in a pod. See the [Direct Block Tools](direct-tools.md#block-storage-tools) topic for more details.

## Teardown

To clean up all the artifacts created by the block demo:

```
kubectl delete -f wordpress.yaml
kubectl delete -f mysql.yaml
kubectl delete -n rook-ceph cephblockpools.ceph.rook.io replicapool
kubectl delete storageclass rook-ceph-block
```

## Flex Driver

To create a volume based on the flex driver instead of the CSI driver, see the following example of a storage class.
Make sure the flex driver is enabled over Ceph CSI.
For this, you need to set `ROOK_ENABLE_FLEX_DRIVER` to `true` in your operator deployment in the `operator.yaml` file.
The pool definition is the same as for the CSI driver.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-ceph-block
provisioner: ceph.rook.io/block
parameters:
  blockPool: replicapool
  # The value of "clusterNamespace" MUST be the same as the one in which your rook cluster exist
  clusterNamespace: rook-ceph
  # Specify the filesystem type of the volume. If not specified, it will use `ext4`.
  fstype: xfs
# Optional, default reclaimPolicy is "Delete". Other options are: "Retain", "Recycle" as documented in https://kubernetes.io/docs/concepts/storage/storage-classes/
reclaimPolicy: Retain
# Optional, if you want to add dynamic resize for PVC. Works for Kubernetes 1.14+
# For now only ext3, ext4, xfs resize support provided, like in Kubernetes itself.
allowVolumeExpansion: true
```

Create the pool and storage class using `kubectl`:

```console
kubectl create -f cluster/examples/kubernetes/ceph/flex/storageclass.yaml
```

Continue with the example above for the [wordpress application](#consume-the-storage-wordpress-sample).

## Advanced Example: Erasure Coded Block Storage

> **IMPORTANT**: This is only possible when using the Flex driver. Ceph CSI 1.2 (with Rook 1.1) does not support this type of configuration yet.

If you want to use erasure coded pool with RBD, your OSDs must use `bluestore` as their `storeType`.
Additionally the nodes that are going to mount the erasure coded RBD block storage must have Linux kernel >= `4.11`.

To be able to use an erasure coded pool you need to create two pools (as seen below in the definitions): one erasure coded and one replicated.
The replicated pool must be specified as the `blockPool` parameter. It is used for the metadata of the RBD images.
The erasure coded pool must be set as the `dataBlockPool` parameter below. It is used for the data of the RBD images.

> **NOTE**: This example requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicated-metadata-pool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ec-data-pool
  namespace: rook-ceph
spec:
  failureDomain: host
  # Make sure you have enough nodes and OSDs running bluestore to support the replica size or erasure code chunks.
  # For the below settings, you need at least 3 OSDs on different nodes (because the `failureDomain` is `host` by default).
  erasureCoded:
    dataChunks: 2
    codingChunks: 1
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-ceph-block
provisioner: ceph.rook.io/block
parameters:
  blockPool: replicated-metadata-pool
  dataBlockPool: ec-data-pool
  # Specify the namespace of the rook cluster from which to create volumes.
  # If not specified, it will use `rook` as the default namespace of the cluster.
  # This is also the namespace where the cluster will be
  clusterNamespace: rook-ceph
  # Specify the filesystem type of the volume. If not specified, it will use `ext4`.
  fstype: xfs
# Works for Kubernetes 1.14+
allowVolumeExpansion: true
```

(These definitions can also be found in the [`storageclass-ec.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/flex/storage-class-ec.yaml) file)
