---
title: Network Filesystem (NFS)
weight: 800
indent: true
---
{% include_relative branch.liquid %}

# Network Filesystem (NFS)

NFS allows remote hosts to mount filesystems over a network and interact with those filesystems as though they are mounted locally. This enables system administrators to consolidate resources onto centralized servers on the network.

## Prerequisites

1. A Kubernetes cluster (v1.16 or higher) is necessary to run the Rook NFS operator. To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).
2. The desired volume to export needs to be attached to the NFS server pod via a [PVC](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims).
Any type of PVC can be attached and exported, such as Host Path, AWS Elastic Block Store, GCP Persistent Disk, CephFS, Ceph RBD, etc.
The limitations of these volumes also apply while they are shared by NFS.
You can read further about the details and limitations of these volumes in the [Kubernetes docs](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).
3. NFS client packages must be installed on all nodes where Kubernetes might run pods with NFS mounted. Install `nfs-utils` on CentOS nodes or `nfs-common` on Ubuntu nodes.

## Deploy NFS Operator

First deploy the Rook NFS operator using the following commands:

```console
$ git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd rook/cluster/examples/kubernetes/nfs
kubectl create -f crds.yaml
kubectl create -f operator.yaml
```

You can check if the operator is up and running with:

```console
kubectl -n rook-nfs-system get pod
```

>```
>NAME                                    READY   STATUS    RESTARTS   AGE
>rook-nfs-operator-879f5bf8b-gnwht       1/1     Running   0          29m
>```

## Deploy NFS Admission Webhook (Optional)

Admission webhooks are HTTP callbacks that receive admission requests to the API server. Two types of admission webhooks is validating admission webhook and mutating admission webhook. NFS Operator support validating admission webhook which validate the NFSServer object sent to the API server before stored in the etcd (persisted).

To enable admission webhook on NFS such as validating admission webhook, you need to do as following:

First, ensure that `cert-manager` is installed.  If it is not installed yet, you can install it as described in the `cert-manager` [installation](https://cert-manager.io/docs/installation/kubernetes/) documentation. Alternatively, you can simply just run the single command below:

```console
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.15.1/cert-manager.yaml
```

This will easily get the latest version (`v0.15.1`) of `cert-manager`  installed. After that completes, make sure the cert-manager component deployed properly and is in the `Running` status:

```console
kubectl get -n cert-manager pod
```

>```
>NAME                                      READY   STATUS    RESTARTS   AGE
>cert-manager-7747db9d88-jmw2f             1/1     Running   0          2m1s
>cert-manager-cainjector-87c85c6ff-dhtl8   1/1     Running   0          2m1s
>cert-manager-webhook-64dc9fff44-5g565     1/1     Running   0          2m1s
>```

Once `cert-manager` is running, you can now deploy the NFS webhook:

```console
kubectl create -f webhook.yaml
```

Verify the webhook is up and running:

```console
kubectl -n rook-nfs-system get pod
```

>```
>NAME                                    READY   STATUS    RESTARTS   AGE
>rook-nfs-operator-78d86bf969-k7lqp      1/1     Running   0          102s
>rook-nfs-webhook-74749cbd46-6jw2w       1/1     Running   0          102s
>```

## Create Openshift Security Context Constraints (Optional)

On OpenShift clusters, we will need to create some additional security context constraints.  If you are **not** running in OpenShift you can skip this and go to the [next section](#create-and-initialize-nfs-server).

To create the security context constraints for nfs-server pods, we can use the following yaml, which is also found in `scc.yaml` under `/cluster/examples/kubernetes/nfs`.

> *NOTE: Older versions of OpenShift may require ```apiVersion: v1```*

```yaml
kind: SecurityContextConstraints
apiVersion: security.openshift.io/v1
metadata:
  name: rook-nfs
allowHostDirVolumePlugin: true
allowHostIPC: false
allowHostNetwork: false
allowHostPID: false
allowHostPorts: false
allowPrivilegedContainer: false
allowedCapabilities:
- SYS_ADMIN
- DAC_READ_SEARCH
defaultAddCapabilities: null
fsGroup:
  type: MustRunAs
priority: null
readOnlyRootFilesystem: false
requiredDropCapabilities:
- KILL
- MKNOD
- SYS_CHROOT
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
supplementalGroups:
  type: RunAsAny
volumes:
- configMap
- downwardAPI
- emptyDir
- persistentVolumeClaim
- secret
users:
  - system:serviceaccount:rook-nfs:rook-nfs-server
```

You can create scc with following command:

```console
oc create -f scc.yaml
```

## Create Pod Security Policies (Recommended)

We recommend you to create Pod Security Policies as well

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: rook-nfs-policy
spec:
  privileged: true
  fsGroup:
    rule: RunAsAny
  allowedCapabilities:
  - DAC_READ_SEARCH
  - SYS_RESOURCE
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
  - configMap
  - downwardAPI
  - emptyDir
  - persistentVolumeClaim
  - secret
  - hostPath
```

Save this file with name `psp.yaml` and create with following command:

```console
kubectl create -f psp.yaml
```

## Create and Initialize NFS Server

Now that the operator is running, we can create an instance of a NFS server by creating an instance of the `nfsservers.nfs.rook.io` resource.
The various fields and options of the NFS server resource can be used to configure the server and its volumes to export.
Full details of the available configuration options can be found in the [NFS CRD documentation](nfs-crd.md).

Before we create NFS Server we need to create `ServiceAccount` and `RBAC` rules

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name:  rook-nfs
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-nfs-server
  namespace: rook-nfs
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-nfs-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "update", "patch"]
  - apiGroups: [""]
    resources: ["services", "endpoints"]
    verbs: ["get"]
  - apiGroups: ["policy"]
    resources: ["podsecuritypolicies"]
    resourceNames: ["rook-nfs-policy"]
    verbs: ["use"]
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups:
    - nfs.rook.io
    resources:
    - "*"
    verbs:
    - "*"
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-nfs-provisioner-runner
subjects:
  - kind: ServiceAccount
    name: rook-nfs-server
     # replace with namespace where provisioner is deployed
    namespace: rook-nfs
roleRef:
  kind: ClusterRole
  name: rook-nfs-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
```

Save this file with name `rbac.yaml` and create with following command:

```console
kubectl create -f rbac.yaml
```

This guide has 3 main examples that demonstrate exporting volumes with a NFS server:

1. [Default StorageClass example](#default-storageclass-example)
1. [XFS StorageClass example](#xfs-storageclass-example)
1. [Rook Ceph volume example](#rook-ceph-volume-example)

### Default StorageClass example

This first example will walk through creating a NFS server instance that exports storage that is backed by the default `StorageClass` for the environment you happen to be running in.
In some environments, this could be a host path, in others it could be a cloud provider virtual disk.
Either way, this example requires a default `StorageClass` to exist.

Start by saving the below NFS CRD instance definition to a file called `nfs.yaml`:

```yaml
---
# A default storageclass must be present
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-default-claim
  namespace: rook-nfs
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: share1
    server:
      accessMode: ReadWrite
      squash: "none"
    # A Persistent Volume Claim must be created before creating NFS CRD instance.
    persistentVolumeClaim:
      claimName: nfs-default-claim
  # A key/value list of annotations
  annotations:
    rook: nfs
```

With the `nfs.yaml` file saved, now create the NFS server as shown:

```console
kubectl create -f nfs.yaml
```

### XFS StorageClass example

Rook NFS support disk quota through `xfs_quota`. So if you need specify disk quota for your volumes you can follow this example.

In this example, we will use an underlying volume mounted as `xfs` with `prjquota` option. Before you can create that underlying volume, you need to create `StorageClass` with `xfs` filesystem and `prjquota` mountOptions. Many distributed storage providers for Kubernetes support `xfs` filesystem. Typically by defining `fsType: xfs` or `fs: xfs` in storageClass parameters. But actually how to specify storage-class filesystem type is depend on the storage providers it self. You can see https://kubernetes.io/docs/concepts/storage/storage-classes/ for more details.

Here is example `StorageClass` for GCE PD and AWS EBS

- GCE PD

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard-xfs
parameters:
  type: pd-standard
  fsType: xfs
mountOptions:
  - prjquota
provisioner: kubernetes.io/gce-pd
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
```

- AWS EBS

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard-xfs
provisioner: kubernetes.io/aws-ebs
parameters:
  type: io1
  iopsPerGB: "10"
  fsType: xfs
mountOptions:
  - prjquota
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

Once you already have `StorageClass` with `xfs` filesystem and `prjquota` mountOptions you can create NFS server instance with the following example.

```yaml
---
# A storage class with name standard-xfs must be present.
# The storage class must be has xfs filesystem type  and prjquota mountOptions.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-xfs-claim
  namespace: rook-nfs
spec:
  storageClassName: "standard-xfs"
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: share1
    server:
      accessMode: ReadWrite
      squash: "none"
    # A Persistent Volume Claim must be created before creating NFS CRD instance.
    persistentVolumeClaim:
      claimName: nfs-xfs-claim
  # A key/value list of annotations
  annotations:
    rook: nfs
```

Save this PVC and NFS Server instance as `nfs-xfs.yaml` and create with following command.

```console
kubectl create -f nfs-xfs.yaml
```

### Rook Ceph volume example

In this alternative example, we will use a different underlying volume as an export for the NFS server.
These steps will walk us through exporting a Ceph RBD block volume so that clients can access it across the network.

First, you have to [follow these instructions](ceph-quickstart.md) to deploy a sample Rook Ceph cluster that can be attached to the NFS server pod for sharing.
After the Rook Ceph cluster is up and running, we can create proceed with creating the NFS server.

Save this PVC and NFS Server instance as `nfs-ceph.yaml`:

```yaml
---
# A rook ceph cluster must be running
# Create a rook ceph cluster using examples in rook/cluster/examples/kubernetes/ceph
# Refer to https://rook.io/docs/rook/master/ceph-quickstart.html for a quick rook cluster setup
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-ceph-claim
  namespace: rook-nfs
spec:
  storageClassName: rook-ceph-block
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 2Gi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: share1
    server:
      accessMode: ReadWrite
      squash: "none"
    # A Persistent Volume Claim must be created before creating NFS CRD instance.
    # Create a Ceph cluster for using this example
    # Create a ceph PVC after creating the rook ceph cluster using ceph-pvc.yaml
    persistentVolumeClaim:
      claimName: nfs-ceph-claim
  # A key/value list of annotations
  annotations:
    rook: nfs
```

Create the NFS server instance that you saved in `nfs-ceph.yaml`:

```console
kubectl create -f nfs-ceph.yaml
```

### Verify NFS Server

We can verify that a Kubernetes object has been created that represents our new NFS server and its export with the command below.

```console
kubectl -n rook-nfs get nfsservers.nfs.rook.io
```

>```
>NAME       AGE   STATE
>rook-nfs   32s   Running
>```

Verify that the NFS server pod is up and running:

```console
kubectl -n rook-nfs get pod -l app=rook-nfs
```

>```
>NAME         READY     STATUS    RESTARTS   AGE
>rook-nfs-0   1/1       Running   0          2m
>```

If the NFS server pod is in the `Running` state, then we have successfully created an exported NFS share that clients can start to access over the network.


## Accessing the Export

Since Rook version v1.0, Rook supports dynamic provisioning of NFS.
This example will be showing how dynamic provisioning feature can be used for nfs.

Once the NFS Operator and an instance of NFSServer is deployed. A storageclass similar to below example has to be created to dynamically provisioning volumes.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  labels:
    app: rook-nfs
  name: rook-nfs-share1
parameters:
  exportName: share1
  nfsServerName: rook-nfs
  nfsServerNamespace: rook-nfs
provisioner: nfs.rook.io/rook-nfs-provisioner
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

You can save it as a file, eg: called `sc.yaml` Then create storageclass with following command.

```console
kubectl create -f sc.yaml
```

> **NOTE**: The StorageClass need to have the following 3 parameters passed.
>
1. `exportName`: It tells the provisioner which export to use for provisioning the volumes.
2. `nfsServerName`: It is the name of the NFSServer instance.
3. `nfsServerNamespace`: It namespace where the NFSServer instance is running.

Once the above storageclass has been created, you can create a PV claim referencing the storageclass as shown in the example given below.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rook-nfs-pv-claim
spec:
  storageClassName: "rook-nfs-share1"
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Mi
```

You can also save it as a file, eg: called `pvc.yaml` Then create PV claim with following command.

```console
kubectl create -f pvc.yaml
```

## Consuming the Export

Now we can consume the PV that we just created by creating an example web server app that uses the above `PersistentVolumeClaim` to claim the exported volume.
There are 2 pods that comprise this example:

1. A web server pod that will read and display the contents of the NFS share
1. A writer pod that will write random data to the NFS share so the website will continually update

Start both the busybox pod (writer) and the web server from the `cluster/examples/kubernetes/nfs` folder:

```console
kubectl create -f busybox-rc.yaml
kubectl create -f web-rc.yaml
```

Let's confirm that the expected busybox writer pod and web server pod are **all** up and in the `Running` state:

```console
kubectl get pod -l app=nfs-demo
```

In order to be able to reach the web server over the network, let's create a service for it:

```console
kubectl create -f web-service.yaml
```

We can then use the busybox writer pod we launched before to check that nginx is serving the data appropriately.
In the below 1-liner command, we use `kubectl exec` to run a command in the busybox writer pod that uses `wget` to retrieve the web page that the web server pod is hosting. As the busybox writer pod continues to write a new timestamp, we should see the returned output also update every ~10 seconds or so.

```console
$ echo; kubectl exec $(kubectl get pod -l app=nfs-demo,role=busybox -o jsonpath='{.items[0].metadata.name}') -- wget -qO- http://$(kubectl get services nfs-web -o jsonpath='{.spec.clusterIP}'); echo
```

>```
>Thu Oct 22 19:28:55 UTC 2015
>nfs-busybox-w3s4t
>```

## Teardown

To clean up all resources associated with this walk-through, you can run the commands below.

```console
kubectl delete -f web-service.yaml
kubectl delete -f web-rc.yaml
kubectl delete -f busybox-rc.yaml
kubectl delete -f pvc.yaml
kubectl delete -f pv.yaml
kubectl delete -f nfs.yaml
kubectl delete -f nfs-xfs.yaml
kubectl delete -f nfs-ceph.yaml
kubectl delete -f rbac.yaml
kubectl delete -f psp.yaml
kubectl delete -f scc.yaml # if deployed
kubectl delete -f operator.yaml
kubectl delete -f webhook.yaml # if deployed
kubectl delete -f crds.yaml
```

## Troubleshooting

If the NFS server pod does not come up, the first step would be to examine the NFS operator's logs:

```console
kubectl -n rook-nfs-system logs -l app=rook-nfs-operator
```
