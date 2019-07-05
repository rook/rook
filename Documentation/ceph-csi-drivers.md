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
   guide](https://github.com/rook/rook/blob/v1.0/Documentation/ceph-quickstart.md))

## CSI Drivers Enablement

### Create RBAC used by CSI drivers in the same namespace as Rook Ceph Operator

```console
# create rbac. Since rook operator is not permitted to create rbac rules,
# these rules have to be created outside of operator
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
NAME                                       READY   STATUS      RESTARTS   AGE
pod/csi-cephfsplugin-nd5tv                 2/2     Running     1          4m5s
pod/csi-cephfsplugin-provisioner-0         2/2     Running     0          4m5s
pod/csi-rbdplugin-provisioner-0            4/4     Running     1          4m5s
pod/csi-rbdplugin-wr78j                    2/2     Running     1          4m5s
pod/rook-ceph-agent-bf772                  1/1     Running     0          7m57s
pod/rook-ceph-mgr-a-7f86bb4968-wdd4l       1/1     Running     0          5m28s
pod/rook-ceph-mon-a-648b78fc99-jthsz       1/1     Running     0          6m1s
pod/rook-ceph-mon-b-6f55c9b6fc-nlp4r       1/1     Running     0          5m55s
pod/rook-ceph-mon-c-69f4f466d5-4q2jk       1/1     Running     0          5m45s
pod/rook-ceph-operator-7464bd774c-scb5c    1/1     Running     0          4m7s
pod/rook-ceph-osd-0-7bfdf45977-n5tt9       1/1     Running     0          2m8s
pod/rook-ceph-osd-1-88f95577d-27jk4        1/1     Running     0          2m8s
pod/rook-ceph-osd-2-674b4dcd4c-5wzz9       1/1     Running     0          2m8s
pod/rook-ceph-osd-3-58f6467f6b-q5wwf       1/1     Running     0          2m8s
pod/rook-discover-6t644                    1/1     Running     0          7m57s

NAME                                   TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/csi-cephfsplugin-provisioner   ClusterIP   10.100.46.135    <none>        1234/TCP   4m5s
service/csi-rbdplugin-provisioner      ClusterIP   10.110.210.40    <none>        1234/TCP   4m5s
service/rook-ceph-mgr                  ClusterIP   10.104.191.254   <none>        9283/TCP   5m13s
service/rook-ceph-mgr-dashboard        ClusterIP   10.97.152.26     <none>        8443/TCP   5m13s
service/rook-ceph-mon-a                ClusterIP   10.108.83.214    <none>        6789/TCP   6m4s
service/rook-ceph-mon-b                ClusterIP   10.104.64.44     <none>        6789/TCP   5m56s
service/rook-ceph-mon-c                ClusterIP   10.103.170.196   <none>        6789/TCP   5m45s

NAME                              DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/csi-cephfsplugin   1         1         1       1            1           <none>          4m5s
daemonset.apps/csi-rbdplugin      1         1         1       1            1           <none>          4m5s
daemonset.apps/rook-ceph-agent    1         1         1       1            1           <none>          7m57s
daemonset.apps/rook-discover      1         1         1       1            1           <none>          7m57s

NAME                                 READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/rook-ceph-mgr-a      1/1     1            1           5m28s
deployment.apps/rook-ceph-mon-a      1/1     1            1           6m2s
deployment.apps/rook-ceph-mon-b      1/1     1            1           5m55s
deployment.apps/rook-ceph-mon-c      1/1     1            1           5m45s
deployment.apps/rook-ceph-operator   1/1     1            1           10m
deployment.apps/rook-ceph-osd-0      1/1     1            1           2m8s
deployment.apps/rook-ceph-osd-1      1/1     1            1           2m8s
deployment.apps/rook-ceph-osd-2      1/1     1            1           2m8s
deployment.apps/rook-ceph-osd-3      1/1     1            1           2m8s

NAME                                            DESIRED   CURRENT   READY   AGE
replicaset.apps/rook-ceph-mgr-a-7f86bb4968      1         1         1       5m28s
replicaset.apps/rook-ceph-mon-a-648b78fc99      1         1         1       6m1s
replicaset.apps/rook-ceph-mon-b-6f55c9b6fc      1         1         1       5m55s
replicaset.apps/rook-ceph-mon-c-69f4f466d5      1         1         1       5m45s
replicaset.apps/rook-ceph-operator-6c49994c4f   0         0         0       10m
replicaset.apps/rook-ceph-operator-7464bd774c   1         1         1       4m7s
replicaset.apps/rook-ceph-osd-0-7bfdf45977      1         1         1       2m8s
replicaset.apps/rook-ceph-osd-1-88f95577d       1         1         1       2m8s
replicaset.apps/rook-ceph-osd-2-674b4dcd4c      1         1         1       2m8s
replicaset.apps/rook-ceph-osd-3-58f6467f6b      1         1         1       2m8s

NAME                                            READY   AGE
statefulset.apps/csi-cephfsplugin-provisioner   1/1     4m5s
statefulset.apps/csi-rbdplugin-provisioner      1/1     4m5s
```

Once the plugin is successfully deployed, test it by running the following example.

# Test RBD CSI Driver

## Create RBD StorageClass

This
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml)
expects a pool named `rbd` in your Ceph cluster. You can create this pool using
[rook pool
CRD](https://github.com/rook/rook/blob/v1.0/Documentation/ceph-pool-crd.md).

Please update `monitors` to reflect the Ceph monitors.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml
```

## Create RBD Secret

Create a Secret that matches `adminid` or `userid` specified in the
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml).

Find a Ceph operator pod (in the following example, the pod is
`rook-ceph-operator-7464bd774c-scb5c`) and create a Ceph user for that pool called
`kubernetes`:

```bash
kubectl exec -ti -n rook-ceph rook-ceph-operator-7464bd774c-scb5c -- bash -c "ceph -c /var/lib/rook/rook-ceph/rook-ceph.config auth get-or-create-key client.kubernetes mon \"allow profile rbd\" osd \"profile rbd pool=rbd\""
```

Then create a Secret using admin and `kubernetes` keyrings:

In [secret](../cluster/examples/kubernetes/ceph/csi/example/rbd/secret.yaml),
you need your Ceph admin/user password encoded in base64.

Run `ceph auth ls` in your rook ceph operator pod, to encode the key of your
admin/user run `echo -n KEY|base64`
and replace `BASE64-ENCODED-PASSWORD` by your encoded key.

```bash
kubectl exec -ti -n rook-ceph rook-ceph-operator-6c49994c4f-pwqcx /bin/sh
sh-4.2# ceph auth ls
installed auth entries:

osd.0
	key: AQA3pa1cN/fODBAAc/jIm5IQDClm+dmekSmSlg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.1
	key: AQBXpa1cTjuYNRAAkohlInoYAa6A3odTRDhnAg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.2
	key: AQB4pa1cvJidLRAALZyAtuOwArO8JZfy7Y5pFg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.3
	key: AQCcpa1cFFQRHRAALBYhqO3m0FRA9pxTOFT2eQ==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
client.admin
	key: AQD0pK1cqcBDCBAAdXNXfgAambPz5qWpsq0Mmw==
	auid: 0
	caps: [mds] allow *
	caps: [mgr] allow *
	caps: [mon] allow *
	caps: [osd] allow *
client.bootstrap-mds
	key: AQD6pK1crJyZCxAA1UTGwtyFv3YYFcBmhWHyoQ==
	caps: [mon] allow profile bootstrap-mds
client.bootstrap-mgr
	key: AQD6pK1c2KaZCxAATWi/I3i0/XEesSipy/HeIA==
	caps: [mon] allow profile bootstrap-mgr
client.bootstrap-osd
	key: AQD6pK1cwa+ZCxAA7XKXRyLQpaHZ+lRXeUk8xQ==
	caps: [mon] allow profile bootstrap-osd
client.bootstrap-rbd
	key: AQD6pK1cULmZCxAA4++Ch/iRKa52297/rbHP+w==
	caps: [mon] allow profile bootstrap-rbd
client.bootstrap-rgw
	key: AQD6pK1cbMKZCxAAGKj5HaMoEl41LHqEafcfPA==
	caps: [mon] allow profile bootstrap-rgw
mgr.a
	key: AQAZpa1chl+DAhAAYyolLBrkht+0sH0HljkFIg==
	caps: [mds] allow *
	caps: [mon] allow *
	caps: [osd] allow *

#encode admin/user key
sh-4.2# echo -n AQD0pK1cqcBDCBAAdXNXfgAambPz5qWpsq0Mmw==|base64
QVFEMHBLMWNxY0JEQ0JBQWRYTlhmZ0FhbWJQejVxV3BzcTBNbXc9PQ==
#or
sh-4.2# ceph auth get-key client.admin|base64
QVFEMHBLMWNxY0JEQ0JBQWRYTlhmZ0FhbWJQejVxV3BzcTBNbXc9PQ==
```

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/secret.yaml
```

## Create RBD PersistentVolumeClaim

Make sure your `storageClassName` is the name of the StorageClass previously
defined in storageclass.yaml

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/pvc.yaml
```

## Verify RBD PVC has successfully been created

```yaml
# kubectl get pvc
NAME      STATUS   VOLUME                 CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rbd-pvc   Bound    pvc-c20495c0d5de11e8   1Gi        RWO            csi-rbd        21s
```

If your PVC status isn't `Bound`, check the csi-rbdplugin logs to see what's
preventing the PVC from being up and bound.

## Create RBD demo Pod

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/pod.yaml
```

When running `rbd list block --pool [yourpool]` in one of your Ceph pods you
should see the created PVC:

```bash
# rbd list block --pool rbd
pvc-c20495c0d5de11e8
```

## Additional features

### RBD Snapshots

Since this feature is still in [alpha
stage](https://kubernetes.io/blog/2018/10/09/introducing-volume-snapshot-alpha-for-kubernetes/)
(k8s 1.12+), make sure to enable `VolumeSnapshotDataSource` feature gate in
your Kubernetes cluster.

#### create RBD snapshot-class

You need to create the `SnapshotClass`. The purpose of a `SnapshotClass` is
defined in [the kubernetes
documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/).
In short, as the documentation describes it:
> Just like StorageClass provides a way for administrators to describe the
> “classes” of storage they offer when provisioning a volume,
> VolumeSnapshotClass provides a way to describe the “classes” of storage when
> provisioning a volume snapshot.

In [snapshotClass](cluster/examples/kubernetes/ceph/csi/example/rbd/snapshotclass.yaml),
the `csi.storage.k8s.io/snapshotter-secret-name` parameter should reference the
name of the secret created for the rbdplugin. The `monitors` are a comma
separated list of your Ceph monitors and `pool` to reflect the Ceph pool name.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/snapshotclass.yaml
```

#### create volumesnapshot

In [snapshot](cluster/examples/kubernetes/ceph/csi/example/rbd/snapshot.yaml),
`snapshotClassName` should be the name of the `VolumeSnapshotClass` previously
created. The source name should be the name of the PVC you created earlier.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/snapshot.yaml
```

#### Verify RBD Snapshot has successfully been created

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

#### Restore the snapshot to a new PVC

In
[pvc-restore](cluster/examples/kubernetes/ceph/csi/example/rbd/pvc-restore.yaml),
`dataSource` should be the name of the `VolumeSnapshot` previously
created. The kind should be the `VolumeSnapshot`.

Create a new PVC from the snapshot

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/rbd/pvc-restore.yaml
```

#### Verify RBD clone PVC has successfully been created

```yaml
# kubectl get pvc
NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rbd-pvc           Bound    pvc-84294e34-577a-11e9-b34f-525400581048   1Gi        RWO            csi-rbd        34m
rbd-pvc-restore   Bound    pvc-575537bf-577f-11e9-b34f-525400581048   1Gi        RWO            csi-rbd        8s
```

## RBD resource Cleanup

To clean your cluster of the resources created by this example, run the following:

if you have tested snapshot, delete snapshotclass, snapshot and pvc-restore
created to test snapshot feature

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/pvc-restore.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/snapshot.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/snapshotclass.yaml
```

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/pod.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/pvc.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/secret.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/rbd/storageclass.yaml
```

# Test CephFS CSI Driver

## Create CephFS StorageClass

This
[storageclass](../cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml)
expect a pool named `cephfs_data` in your Ceph cluster. You can create this
pool using [rook file-system
CRD](https://github.com/rook/rook/blob/v1.0/Documentation/ceph-filesystem-crd.md).

Please update `monitors` to reflect the Ceph monitors.

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml
```

## Create CephFS Secret

Create a Secret that matches `provisionVolume` type specified in the [storageclass](../cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml).

In [secret](../cluster/examples/kubernetes/ceph/csi/example/cephfs/secret.yaml)
you need your Ceph admin/user ID and password encoded in base64. Encode
admin/user ID in base64 format and replace `BASE64-ENCODED-USER`.

```bash
$echo -n admin|base64
YWRtaW4=
```

Run `ceph auth ls` in your rook ceph operator pod, to encode the key of your
admin/user run `echo -n KEY|base64`
and replace `BASE64-ENCODED-PASSWORD` by your encoded key.

```bash
kubectl exec -ti -n rook-ceph rook-ceph-operator-6c49994c4f-pwqcx /bin/sh
sh-4.2# ceph auth ls
installed auth entries:

osd.0
	key: AQA3pa1cN/fODBAAc/jIm5IQDClm+dmekSmSlg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.1
	key: AQBXpa1cTjuYNRAAkohlInoYAa6A3odTRDhnAg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.2
	key: AQB4pa1cvJidLRAALZyAtuOwArO8JZfy7Y5pFg==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
osd.3
	key: AQCcpa1cFFQRHRAALBYhqO3m0FRA9pxTOFT2eQ==
	caps: [mgr] allow profile osd
	caps: [mon] allow profile osd
	caps: [osd] allow *
client.admin
	key: AQD0pK1cqcBDCBAAdXNXfgAambPz5qWpsq0Mmw==
	auid: 0
	caps: [mds] allow *
	caps: [mgr] allow *
	caps: [mon] allow *
	caps: [osd] allow *
client.bootstrap-mds
	key: AQD6pK1crJyZCxAA1UTGwtyFv3YYFcBmhWHyoQ==
	caps: [mon] allow profile bootstrap-mds
client.bootstrap-mgr
	key: AQD6pK1c2KaZCxAATWi/I3i0/XEesSipy/HeIA==
	caps: [mon] allow profile bootstrap-mgr
client.bootstrap-osd
	key: AQD6pK1cwa+ZCxAA7XKXRyLQpaHZ+lRXeUk8xQ==
	caps: [mon] allow profile bootstrap-osd
client.bootstrap-rbd
	key: AQD6pK1cULmZCxAA4++Ch/iRKa52297/rbHP+w==
	caps: [mon] allow profile bootstrap-rbd
client.bootstrap-rgw
	key: AQD6pK1cbMKZCxAAGKj5HaMoEl41LHqEafcfPA==
	caps: [mon] allow profile bootstrap-rgw
mgr.a
	key: AQAZpa1chl+DAhAAYyolLBrkht+0sH0HljkFIg==
	caps: [mds] allow *
	caps: [mon] allow *
	caps: [osd] allow *

#encode admin/user key
sh-4.2#echo -n AQD0pK1cqcBDCBAAdXNXfgAambPz5qWpsq0Mmw==|base64
QVFEMHBLMWNxY0JEQ0JBQWRYTlhmZ0FhbWJQejVxV3BzcTBNbXc9PQ==
#or
sh-4.2#ceph auth get-key client.admin|base64
QVFEMHBLMWNxY0JEQ0JBQWRYTlhmZ0FhbWJQejVxV3BzcTBNbXc9PQ==
```

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/cephfs/secret.yaml
```

## Create CephFS PersistentVolumeClaim

In [pvc](../cluster/examples/kubernetes/ceph/csi/example/cephfs/pvc.yaml),
make sure your `storageClassName` is the name of the `StorageClass` previously
defined in `storageclass.yaml`

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/cephfs/pvc.yaml
```

## Verify CephFS PVC has successfully been created

```bash
# kubectl get pvc
NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
cephfs-pvc   Bound    pvc-6bc76846-3a4a-11e9-971d-525400c2d871   1Gi        RWO            csi-cephfs     25s

```

If your PVC status isn't `Bound`, check the csi-cephfsplugin logs to see what's
preventing the PVC from being up and bound.

## Create CephFS demo Pod

```console
kubectl create -f cluster/examples/kubernetes/ceph/csi/example/cephfs/pod.yaml
```

Once the PVC is attached to the pod, pod creation process will continue

## CephFS resource Cleanup

To clean your cluster of the resources created by this example, run the
following:

```console
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/cephfs/pod.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/cephfs/pvc.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/cephfs/secret.yaml
kubectl delete -f cluster/examples/kubernetes/ceph/csi/example/cephfs/storageclass.yaml
```
