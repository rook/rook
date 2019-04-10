---
title: Ceph CSI
weight: 3200
indent: true
---

# Running Ceph CSI drivers with Rook

Here is a guide on how to use Rook to deploy ceph-csi drivers on a Kubernetes
cluster.

- [Enable CSI drivers](#csi-drivers-enablement)
- [Test RBD CSI driver](#Test-RBD-CSI-Driver)
- [Test CephFS CSI driver](#Test-CephFs-CSI-Driver)

## Prerequisites

1. a Kubernetes v1.13+ is needed in order to support CSI Spec 1.0.
2. `--allow-privileged` flag set to true in
   [kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/)
   and your [API
   server](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/)
3. An up and running Rook instance (see [Rook - Ceph quickstart
   guide](https://github.com/rook/rook/blob/master/Documentation/ceph-quickstart.md))

## CSI Drivers Enablement

### Create RBAC used by CSI drivers in the same namespace as Rook Ceph Operator

```console
#check if rook-ceph namespace is already created, if not create it
kubectl get ns
kubectl create namespace rook-ceph
# create rbac. Since rook operator is not permitted to create rbac rules,
# these rules have to be created outside of operator
kubectl create -f cluster/examples/kubernetes/ceph/common.yaml
kubectl apply -f cluster/examples/kubernetes/ceph/csi/rbac/rbd/
kubectl apply -f cluster/examples/kubernetes/ceph/csi/rbac/cephfs/
```

### Start Rook Ceph Operator

```console
kubectl apply -f cluster/examples/kubernetes/ceph/operator-with-csi.yaml
```

### Verify CSI drivers and Operator are up and running

```bash
# kubectl get all -n rook-ceph
NAME                                     READY     STATUS    RESTARTS   AGE
pod/csi-cephfsplugin-h5spd               2/2       Running   0          1d
pod/csi-cephfsplugin-provisioner-0       2/2       Running   0          1d
pod/csi-rbdplugin-4l6zg                  2/2       Running   2          1d
pod/csi-rbdplugin-provisioner-0          4/4       Running   2          1d
pod/rook-ceph-agent-zlm84                1/1       Running   0          1d
pod/rook-ceph-operator-c84954957-jdzk6   1/1       Running   0          1d
pod/rook-discover-66hjp                  1/1       Running   0          1d

NAME                                   TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)    AGE
service/csi-cephfsplugin-provisioner   ClusterIP   10.0.0.107   <none>        1234/TCP   1d
service/csi-rbdplugin-provisioner      ClusterIP   10.0.0.56    <none>        1234/TCP   1d

NAME                              DESIRED   CURRENT   READY     UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/csi-cephfsplugin   1         1         1         1            1           <none>          1d
daemonset.apps/csi-rbdplugin      1         1         1         1            1           <none>          1d
daemonset.apps/rook-ceph-agent    1         1         1         1            1           <none>          1d
daemonset.apps/rook-discover      1         1         1         1            1           <none>          1d

NAME                                 DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/rook-ceph-operator   1         1         1            1           1d

NAME                                           DESIRED   CURRENT   READY     AGE
replicaset.apps/rook-ceph-operator-c84954957   1         1         1         1d

NAME                                            DESIRED   CURRENT   AGE
statefulset.apps/csi-cephfsplugin-provisioner   1         1         1d
statefulset.apps/csi-rbdplugin-provisioner      1         1         1d
```

Once the plugin is successfully deployed, test it by running the following example.

# Test RBD CSI Driver

## Create RBD StorageClass

This
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml)
expects a pool named `rbd` in your Ceph cluster. You can create this pool using
[rook pool
CRD](https://github.com/rook/rook/blob/master/Documentation/ceph-pool-crd.md).

Please update `monitors` to reflect the Ceph monitors.

## Create RBD Secret

Create a Secret that matches `adminid` or `userid` specified in the
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml).

Find a Ceph mon pod (in the following example, the pod is
`rook-ceph-mon-a-6c4f9f6b6-rzp6r`) and create a Ceph user for that pool called
`kubernetes`:

```bash
kubectl exec -ti -n rook-ceph rook-ceph-mon-a-6c4f9f6b6-rzp6r -- bash -c "ceph -c /var/lib/rook/rook-ceph/rook-ceph.config auth get-or-create-key client.kubernetes mon \"allow profile rbd\" osd \"profile rbd pool=rbd\""
```

Then create a Secret using admin and `kubernetes` keyrings:

## rbd-secret.yaml

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-rbd-secret
  namespace: default
data:
  # Key value corresponds to a user name defined in Ceph cluster
  admin: BASE64-ENCODED-PASSWORD
  # Key value corresponds to a user name defined in Ceph cluster
  kubernetes: BASE64-ENCODED-PASSWORD
  # if monValueFromSecret is set to "monitors", uncomment the
  # following and set the mon there
  #monitors: BASE64-ENCODED-Comma-Delimited-Mons
```

Here, you need your Ceph admin/user password encoded in base64. Run `ceph auth
ls` in one of your Ceph pods, encode the key of your admin/user and replace
`BASE64-ENCODED-PASSWORD` by your encoded key.

## Create RBD PersistentVolumeClaim

### rbd-pvc.yaml

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rbd-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-rbd
```

Make sure your `storageClassName` is the name of the StorageClass previously
defined in storageclass.yaml

## Verify RBD PVC has successfully been created

```yaml
# kubectl get pvc
NAME      STATUS   VOLUME                 CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rbd-pvc   Bound    pvc-c20495c0d5de11e8   1Gi        RWO            csi-rbd        21s
```

If your PVC status isn't `Bound`, check the csi-rbdplugin logs to see what's
preventing the PVC from being up and bound.

## Create RBD demo Pod

### rbd-pod.yaml

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: csirbd-demo-pod
spec:
  containers:
   - name: web-server
     image: nginx
     volumeMounts:
       - name: mypvc
         mountPath: /var/lib/www/html
  volumes:
   - name: mypvc
     persistentVolumeClaim:
       claimName: rbd-pvc
       readOnly: false
```

When running `rbd list block --pool [yourpool]` in one of your Ceph pods you
should see the created PVC:

```bash
# rbd list block --pool rbd
pvc-c20495c0d5de11e8
```

## Additional features

### RBD Snapshots

This example is based on
[kubernetes-csi/external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter),
with a few tweaks to make it work along with the ceph-csi RBD plugin. This is a basic
example of the kubernetes snapshot feature. For more information and
functionalities please refer to the [volume snapshot
documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshots/).

Since this feature is still in [alpha
stage](https://kubernetes.io/blog/2018/10/09/introducing-volume-snapshot-alpha-for-kubernetes/)
(k8s 1.12+), make sure to enable `VolumeSnapshotDataSource` feature gate in
your Kubernetes cluster.

### Enable RBD csi-snapshotter

First, create RBAC rules to authorize the snapshotter to access the needed
resources

```console
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter-rbac.yaml
```

Then, deploy the csi-snapshotter service. This file is based on
[setup-csi-snapshotter.yaml](https://github.com/kubernetes-csi/external-snapshotter/blob/master/deploy/kubernetes/setup-csi-snapshotter.yaml)
without the csi-provisioner and hostpath-plugin containers that are given as an
example. The `volumes` part has been modified to match the ceph-csi plugin
socket path.

If you followed this guide without changing anything, this file should be left
as is. If you made modifications like changing the socket-dir in the plugin
deployment, you must edit this file to match your configuration.

```console
kubectl create -f  https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter.yaml
```

### Test RBD csi-snapshotter

Next you need to create the SnapshotClass. The purpose of a SnapshotClass is
defined in [the kubernetes
documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/).
In short, as the documentation describes it:
> Just like StorageClass provides a way for administrators to describe the
> “classes” of storage they offer when provisioning a volume,
> VolumeSnapshotClass provides a way to describe the “classes” of storage when
> provisioning a volume snapshot.

You must download this file and modify it to match your Ceph cluster.

```console
wget https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshotclass.yaml
```

The `csiSnapshotterSecretName` parameter should reference the name of the
secret created for the ceph-csi plugin you deployed. The monitors are a comma
separated list of your Ceph monitors, same as in the StorageClass of the plugin
you chosen. When this is done, run:

```console
kubectl create -f snapshotclass.yaml
```

Finally, create the VolumeSnapshot resource. its `snapshotClassName` should be
the name of the VolumeSnapshotClass previously created. The source name should
be the name of the PVC you created earlier.

```console
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshot.yaml
```

### Verify RBD Snapshot has successfully been created

```bash
# kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
# kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```

In one of your Ceph pod, run `rbd snap ls [name-of-your-pvc]`.
The output should be similar to this:

```bash
# rbd snap ls pvc-c20495c0d5de11e8
SNAPID NAME                                                                      SIZE TIMESTAMP
     4 csi-rbd-pvc-c20495c0d5de11e8-snap-4c0b455b-d5fe-11e8-bebb-525400123456 1024 MB Mon Oct 22 13:28:03 2018
```

## RBD resource Cleanup

To clean your cluster of the resources created by this example, run the following:

```console
kubectl delete -f rbd-pod.yaml
kubectl delete -f rbd-pvc.yaml
kubectl delete -f rbd-secret.yaml
kubectl delete -f rbd-storageclass.yaml
```

If you tested snapshots too:

```console
kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshot.yaml
kubectl delete -f snapshotclass.yaml
kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter.yaml
kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter-rbac.yaml
```

# Test CephFS CSI Driver

## Create CephFS StorageClass

This
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml)
expect a pool named `cephfs_data` in your Ceph cluster. You can create this
pool using [rook file-system
CRD](https://github.com/rook/rook/blob/master/Documentation/ceph-filesystem-crd.md).

Please update `monitors` to reflect the Ceph monitors.

## Create CephFS Secret

Create a Secret that matches `provisionVolume` type specified in the [storageclass](../cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml).

### cephfs-secret.yaml

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-cephfs-secret
  namespace: default
data:
  # Required if provisionVolume is set to false
  userID: BASE64-ENCODED-USER
  userKey: BASE64-ENCODED-PASSWORD

  # Required if provisionVolume is set to true
  adminID: BASE64-ENCODED-USER
  adminKey: BASE64-ENCODED-PASSWORD
```

Here, you need your Ceph admin/user ID and password encoded in base64.  Encode
admin/user ID in base64 format and replace `BASE64-ENCODED-USER` and Run `ceph
auth ls` in one of your Ceph pods, encode the key of your admin/user and replace
`BASE64-ENCODED-PASSWORD` by your encoded key.

## Create CephFS PersistentVolumeClaim

### cephfs-pvc.yaml

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cephfs-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-cephfs
```

Make sure your `storageClassName` is the name of the StorageClass previously
defined in storageclass.yaml

## Verify CephFS PVC has successfully been created

```bash
# kubectl get pvc
NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
cephfs-pvc   Bound    pvc-6bc76846-3a4a-11e9-971d-525400c2d871   1Gi        RWO            csi-cephfs     25s

```

If your PVC status isn't `Bound`, check the csi-cephfsplugin logs to see what's
preventing the PVC from being up and bound.

## Create CephFS demo Pod

### cephfs-pod.yaml

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: csicephfs-demo-pod
spec:
  containers:
   - name: web-server
     image: nginx
     volumeMounts:
       - name: mypvc
         mountPath: /var/lib/www/html
  volumes:
   - name: mypvc
     persistentVolumeClaim:
       claimName: cephfs-pvc
       readOnly: false
```

## CephFS resource Cleanup

To clean your cluster of the resources created by this example, run the
following:

```console
kubectl delete -f cephfs-pod.yaml
kubectl delete -f cephfs-pvc.yaml
kubectl delete -f cephfs-secret.yaml
kubectl delete -f cephfs-storageclass.yaml
```
