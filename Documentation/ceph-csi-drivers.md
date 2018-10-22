---
title: Ceph CSI
weight: 32
indent: true
---

# Running ceph CSI drivers with Rook

Here is a guide on how to implement ceph-csi drivers on a Kubernetes cluster with Rook. The way to implement it should change relatively soon as there is an effort to manage deployments of the ceph-csi plugin within Rook directly ([#1385](https://github.com/rook/rook/issues/1385), [#2059](https://github.com/rook/rook/pull/2059)).

- [Enable CSI drivers](#enable-csi-drivers)
- [Test the CSI driver](#test-the-csi-driver)
- [Snapshots](#snapshots)
- [Cleanup](#cleanup)

## Prerequisites

1. A Kubernetes v1.12+ cluster with at least one node
2. `--allow-privileged` flag set to true in [kubelet](https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/) and your [API server](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/)
3. An up and running Rook instance (see [Rook - ceph quickstart guide](https://github.com/rook/rook/blob/master/Documentation/ceph-quickstart.md))

## Enable CSI drivers

As described by the ceph-csi [Cephfs](https://github.com/ceph/ceph-csi/blob/master/docs/deploy-cephfs.md) and [RBD](https://github.com/ceph/ceph-csi/blob/master/docs/deploy-rbd.md) plugins deployment guides, the plugins yaml manifests are located in ceph-csi repository in the [deploy folder](https://github.com/ceph/ceph-csi/tree/master/deploy), and should be deployed as follow:

### Deploy RBACs for node plugins, csi-attacher and csi-provisioner (common to Cephfs and RBD):
```
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-attacher-rbac.yaml
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-provisioner-rbac.yaml
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-nodeplugin-rbac.yaml
```
Those manifests deploy service accounts, cluster roles and cluster role bindings.
### Deploy csi-attacher and csi-provisioner containers:
Deploys stateful sets for external-attacher and external-provisioner sidecar containers.
##### Cephfs

```
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin-attacher.yaml
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml
```
##### RBD
```
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin-attacher.yaml
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml
```
### Deploy the CSI driver:
Deploys a daemon set with two containers: CSI driver-registrar and the driver.
##### Cephfs
```
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin.yaml
```
##### RBD
```
kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin.yaml
```

### Verify the plugin, its attacher and provisioner are running:
You should see an output similar to this when you run `kubectl get all`.
```
# kubectl get all

NAMESPACE   NAME                                       READY   STATUS      RESTARTS   AGE
default     pod/csi-rbdplugin-975c4                    2/2     Running     0          132m
default     pod/csi-rbdplugin-attacher-0               1/1     Running     0          132m
default     pod/csi-rbdplugin-provisioner-0            1/1     Running     0          132m

NAMESPACE   NAME                                TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)         AGE
default     service/csi-rbdplugin-attacher      ClusterIP   10.107.65.227    <none>        12345/TCP       132m
default     service/csi-rbdplugin-provisioner   ClusterIP   10.108.120.159   <none>        12345/TCP       132m

NAMESPACE   NAME                             DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
default     daemonset.apps/csi-rbdplugin     1         1         1       1            1           <none>          132m

NAMESPACE   NAME                                         DESIRED   CURRENT   AGE
default     statefulset.apps/csi-rbdplugin-attacher      1         1         132m
default     statefulset.apps/csi-rbdplugin-provisioner   1         1         132m

```

## Test the CSI driver

Once the plugin is successfully deployed, test it by running the following example

This example is based on the ceph-csi [examples](https://github.com/ceph/ceph-csi/tree/master/examples) directory. It will describe how to test the RBD csi-driver, but it is very similar to run the cephfs one.

### Create the StorageClass:
This storageclass expect a pool named `rbd` in your ceph cluster. You can create this pool using [rook pool CRD](https://github.com/rook/rook/blob/master/Documentation/ceph-pool-crd.md)
##### storageclass.yaml
```
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: csi-rbd
provisioner: csi-rbdplugin 
parameters:
    # Comma separated list of Ceph monitors
    # if using FQDN, make sure csi plugin's dns policy is appropriate.
    monitors: mon1:port,mon2:port,...

    # if "monitors" parameter is not set, driver to get monitors from same
    # secret as admin/user credentials. "monValueFromSecret" provides the
    # key in the secret whose value is the mons
    #monValueFromSecret: "monitors"

    
    # Ceph pool into which the RBD image shall be created
    pool: rbd

    # RBD image format. Defaults to "2".
    imageFormat: "2"

    # RBD image features. Available for imageFormat: "2". CSI RBD currently supports only `layering` feature.
    imageFeatures: layering
    
    # The secrets have to contain Ceph admin credentials.
    csiProvisionerSecretName: csi-rbd-secret
    csiProvisionerSecretNamespace: default
    csiNodePublishSecretName: csi-rbd-secret
    csiNodePublishSecretNamespace: default

    # Ceph users for operating RBD
    adminid: admin
    userid: kubernetes
    # uncomment the following to use rbd-nbd as mounter on supported nodes
    #mounter: rbd-nbd
reclaimPolicy: Delete
```
### Create the Secret:
##### secret.yaml
```
apiVersion: v1
kind: Secret
metadata:
  name: csi-rbd-secret
  namespace: default 
data:
  # Key value corresponds to a user name defined in ceph cluster
  admin: BASE64-ENCODED-PASSWORD
  # Key value corresponds to a user name defined in ceph cluster
  kubernetes: BASE64-ENCODED-PASSWORD
  # if monValueFromSecret is set to "monitors", uncomment the
  # following and set the mon there
  #monitors: BASE64-ENCODED-Comma-Delimited-Mons
```
Here, you need your ceph admin/user password encoded in base64. Run `ceph auth ls` in one of your ceph pod, encode the key of your admin/user and replace `BASE64-ENCODED-PASSWORD` by your encoded key.
### Create the PersistentVolumeClaim:
##### pvc.yaml
```
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
Make sure your `storageClassName` is the name of the StorageClass previously defined in storageclass.yaml

### Verify the PVC has successfully been created:
```
# kubectl get pvc
NAME      STATUS   VOLUME                 CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rbd-pvc   Bound    pvc-c20495c0d5de11e8   1Gi        RWO            csi-rbd        21s
```
If your PVC status isn't `Bound`, check the csi-rbdplugin logs to see what's preventing the PVC from being up and bound.
### Create the demo Pod:
##### pod.yaml
```
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

When running `rbd list block --pool [yourpool]` in one of your ceph pod you should see the created PVC:
```
# rbd list block --pool rbd
pvc-c20495c0d5de11e8
```

# Additional features

### Snapshots
This example is based on [kubernetes-csi/external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter), with a few tweaks to make it work along ceph-csi RBD plugin. This is a basic example of the kubernetes snapshot feature. For more information and functionalities please refer to the [volume snapshot documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshots/).

Since this feature is still in [alpha stage](https://kubernetes.io/blog/2018/10/09/introducing-volume-snapshot-alpha-for-kubernetes/) (k8s 1.12+), make sure to enable `VolumeSnapshotDataSource` feature gate in your Kubernetes cluster.

### Enable csi-snapshotter
First, create RBAC rules to authorize the snapshotter to access the needed resources
```
# kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter-rbac.yaml
```
Then, deploy the csi-snapshotter service. This file is based on [setup-csi-snapshotter.yaml](https://github.com/kubernetes-csi/external-snapshotter/blob/master/deploy/kubernetes/setup-csi-snapshotter.yaml) without the csi-provisioner and hostpath-plugin containers that are given as an example. The `volumes` part has been modified to match the ceph-csi plugin socket path.

If you followed this guide without changing anything, this file should be left as is. If you made modifications like changing the socket-dir in the plugin deployment, you must edit this file to match your configuration.
```
# kubectl create -f  https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter.yaml
```
### Test csi-snapshotter
Next you need to create the SnapshotClass. The purpose of a SnapshotClass is defined in [the kubernetes documentation](https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/). In short, as the documentation describes it:
> Just like StorageClass provides a way for administrators to describe the “classes” of storage they offer when provisioning a volume, VolumeSnapshotClass provides a way to describe the “classes” of storage when provisioning a volume snapshot.

You must download this file and modify it to match your Ceph cluster.
```
# wget https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshotclass.yaml
```
The `csiSnapshotterSecretName` parameter should reference the name of the secret created for the ceph-csi plugin you deployed. The monitors are a comma separated list of your ceph monitors, same as in the StorageClass of the plugin you chosen. When this is done, run:
```
# kubectl create -f snapshotclass.yaml
```

Finally, create the VolumeSnapshot resource. its `snapshotClassName` should be the name of the VolumeSnapshotClass previously created. The source name should be the name of the PVC you created earlier.
```
# kubectl create -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshot.yaml
```
### Verify the Snapshot has successfully been created:
```
# kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
# kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```
In one of your ceph pod, run `rbd snap ls [name-of-your-pvc]`.
The output should be similar to this:
```
# rbd snap ls pvc-c20495c0d5de11e8
SNAPID NAME                                                                      SIZE TIMESTAMP                
     4 csi-rbd-pvc-c20495c0d5de11e8-snap-4c0b455b-d5fe-11e8-bebb-525400123456 1024 MB Mon Oct 22 13:28:03 2018 

```

## Cleanup

To clean your cluster of the resources created by this example, run the following:

```
# kubectl delete -f pod.yaml
# kubectl delete -f pvc.yaml
# kubectl delete -f secret.yaml
# kubectl delete -f storageclass.yaml

# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin.yaml
# Or https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin.yaml  if you deployed cephfs plugin

# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml
# Or https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml

# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-rbdplugin-attacher.yaml
# Or https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/cephfs/kubernetes/csi-cephfsplugin-attacher.yaml

# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-nodeplugin-rbac.yaml
# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-provisioner-rbac.yaml
# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/deploy/rbd/kubernetes/csi-attacher-rbac.yaml
```
If you tested snapshots too:
```
# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/snapshot.yaml
# kubectl delete -f snapshotclass.yaml
# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter.yaml
# kubectl delete -f https://raw.githubusercontent.com/ceph/ceph-csi/master/examples/rbd/csi-snapshotter-rbac.yaml
```
