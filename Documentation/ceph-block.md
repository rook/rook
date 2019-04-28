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

Block storage allows you to mount storage to a single pod. This example shows how to build a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Quickstart](ceph-quickstart.md) guide.

## Provision Storage

Before Rook can start provisioning storage, a StorageClass and its storage pool need to be created. This is needed for Kubernetes to interoperate with Rook for provisioning persistent volumes. For more options on pools, see the documentation on [creating storage pools](ceph-pool-crd.md).

**NOTE** This example requires you to have **at least 3 OSDs each on a different node**.
This is because the `replicated.size: 3` will require at least 3 OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) to `host` (default), each OSD needs to be on a different nodes.

Save this storage class definition as `storageclass.yaml`:

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
```

Create the storage class.
```bash
kubectl create -f storageclass.yaml
```

**NOTE** As [specified by Kubernetes](https://v1-13.docs.kubernetes.io/docs/concepts/storage/persistent-volumes/#retain), when using the `Retain` reclaim policy, the ceph RBD images that back up `PersistentVolume`s will continue to exist even after the PV is deleted, and have to be cleaned up manually using `rbd rm`.

## Consume the storage: Wordpress sample

We create a sample app to consume the block storage provisioned by Rook with the classic wordpress and mysql apps.
Both of these apps will make use of block volumes provisioned by Rook.

Start mysql and wordpress from the `cluster/examples/kubernetes` folder:

```bash
kubectl create -f mysql.yaml
kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```bash
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Once the wordpress and mysql pods are in the `Running` state, get the cluster IP of the wordpress app and enter it in your browser:

```bash
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.

If you are using Minikube, the Wordpress URL can be retrieved with this one-line command:

```console
echo http://$(minikube ip):$(kubectl get service wordpress -o jsonpath='{.spec.ports[0].nodePort}')
```

**NOTE:** When running in a vagrant environment, there will be no external IP address to reach wordpress with.  You will only be able to reach wordpress via the `CLUSTER-IP` from inside the Kubernetes cluster.

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

## Advanced Example: Erasure Coded Block Storage

If you want to use erasure coded pool with RBD, your OSDs must use `bluestore` as their `storeType`.
Additionally the nodes that are going to mount the erasure coded RBD block storage must have Linux kernel >= `4.11`.

To be able to use an erasure coded pool you need to create two pools (as seen below in the definitions): one erasure coded and one replicated.
The replicated pool must be specified as the `blockPool` parameter. It is used for the metadata of the RBD images.
The erasure coded pool must be set as the `dataBlockPool` parameter below. It is used for the data of the RBD images.

**NOTE** This example requires you to have **at least 3 bluestore OSDs each on a different node**.
This is because the below `erasureCoded` chunk settings require at least 3 bluestore OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) to `host` (default), each OSD needs to be on a different nodes.

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
```

(These definitions can also be found in the [`storageclass-ec.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml) file)
