---
title: Block Storage Overview
---

Block storage allows a single pod to mount storage. This guide shows how to create a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](../../Getting-Started/quickstart.md).

## Provision Storage

Before Rook can provision storage, a [`StorageClass`](https://kubernetes.io/docs/concepts/storage/storage-classes) and [`CephBlockPool` CR](../../CRDs/Block-Storage/ceph-block-pool-crd.md) need to be created. This will allow Kubernetes to interoperate with Rook when provisioning persistent volumes.

!!! note
    This sample requires *at least 1 OSD per node*, with each OSD located on *3 different nodes*.

Each OSD must be located on a different node, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `replicated.size` is set to `3`.

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

    # (optional) mapOptions is a comma-separated list of map options.
    # For krbd options refer
    # https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options
    # For nbd options refer
    # https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options
    # mapOptions: lock_on_read,queue_depth=1024

    # (optional) unmapOptions is a comma-separated list of unmap options.
    # For krbd options refer
    # https://docs.ceph.com/docs/master/man/8/rbd/#kernel-rbd-krbd-options
    # For nbd options refer
    # https://docs.ceph.com/docs/master/man/8/rbd-nbd/#options
    # unmapOptions: force

    # RBD image format. Defaults to "2".
    imageFormat: "2"

    # RBD image features. Available for imageFormat: "2". CSI RBD currently supports only `layering` feature.
    imageFeatures: layering

    # The secrets contain Ceph admin credentials.
    csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
    csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
    csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
    csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
    csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
    csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph

    # Specify the filesystem type of the volume. If not specified, csi-provisioner
    # will set default as `ext4`. Note that `xfs` is not recommended due to potential deadlock
    # in hyperconverged settings where the volume is mounted on the same node as the osds.
    csi.storage.k8s.io/fstype: ext4

# Delete the rbd volume when a PVC is deleted
reclaimPolicy: Delete

# Optional, if you want to add dynamic resize for PVC.
# For now only ext3, ext4, xfs resize support provided, like in Kubernetes itself.
allowVolumeExpansion: true
```

If you've deployed the Rook operator in a namespace other than "rook-ceph",
change the prefix in the provisioner to match the namespace
you used. For example, if the Rook operator is running in the namespace "my-namespace" the
provisioner value should be "my-namespace.rbd.csi.ceph.com".

Create the storage class.

```console
kubectl create -f deploy/examples/csi/rbd/storageclass.yaml
```

!!! note
    As [specified by Kubernetes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#retain), when using the `Retain` reclaim policy, any Ceph RBD image that is backed by a `PersistentVolume` will continue to exist even after the `PersistentVolume` has been deleted. These Ceph RBD images will need to be cleaned up manually using `rbd rm`.

## Consume the storage: Wordpress sample

We create a sample app to consume the block storage provisioned by Rook with the classic wordpress and mysql apps.
Both of these apps will make use of block volumes provisioned by Rook.

Start mysql and wordpress from the `deploy/examples` folder:

```console
kubectl create -f mysql.yaml
kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```console
kubectl get pvc
```

!!! example "Example Output: `kubectl get pvc`"
    ```console
    NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
    mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
    wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
    ```

Once the wordpress and mysql pods are in the `Running` state, get the cluster IP of the wordpress app and enter it in your browser:

```console
kubectl get svc wordpress
```

!!! example "Example Output: `kubectl get svc wordpress`"
    ```console
    NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
    wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
    ```

You should see the wordpress app running.

If you are using Minikube, the Wordpress URL can be retrieved with this one-line command:

```console
echo http://$(minikube ip):$(kubectl get service wordpress -o jsonpath='{.spec.ports[0].nodePort}')
```

!!! note
    When running in a vagrant environment, there will be no external IP address to reach wordpress with.  You will only be able to reach wordpress via the `CLUSTER-IP` from inside the Kubernetes cluster.

## Consume the storage: Toolbox

With the pool that was created above, we can also create a block image and mount it directly in a pod. See the [Direct Block Tools](../../Troubleshooting/direct-tools.md#block-storage-tools) topic for more details.

## Teardown

To clean up all the artifacts created by the block demo:

```console
kubectl delete -f wordpress.yaml
kubectl delete -f mysql.yaml
kubectl delete -n rook-ceph cephblockpools.ceph.rook.io replicapool
kubectl delete storageclass rook-ceph-block
```

## Advanced Example: Erasure Coded Block Storage

If you want to use erasure coded pool with RBD, your OSDs must use `bluestore` as their `storeType`.
Additionally the nodes that are going to mount the erasure coded RBD block storage must have Linux kernel >= `4.11`.

!!! attention
    This example requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

To be able to use an erasure coded pool you need to create two pools (as seen below in the definitions): one erasure coded and one replicated.

!!! attention
    This example requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

### Erasure Coded CSI Driver

The erasure coded pool must be set as the `dataPool` parameter in
[`storageclass-ec.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/storage-class-ec.yaml) It is used for the data of the RBD images.

### Erasure Coded Flex Driver

The erasure coded pool must be set as the `dataBlockPool` parameter in
[`storageclass-ec.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/flex/storage-class-ec.yaml). It is used for
the data of the RBD images.
