# Rook on Kubernetes

This example shows how to build a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

## Prerequisites

This example requires a running Kubernetes cluster. You will also need to modify the kubelet service to bind mount `/sbin/modprobe` to allow access to `modprobe`. Access to modprobe is necessary for using the rbd volume plugin (<https://github.com/kubernetes/kubernetes/issues/23924>).
If using RKT, you can allow modprobe by following this [doc](https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md#allow-pods-to-use-rbd-volumes).  

For a quick start, use <https://github.com/rook/coreos-kubernetes/tree/rook-demo> (rook-demo branch in the rook fork of the coreos-kubernetes repo). This will bring up a multi-node Kubernetes cluster, configured for using the rbd volume plugin.

```
$ git clone https://github.com/rook/coreos-kubernetes.git
$ cd coreos-kubernetes
$ git checkout rook-demo
```

You can now get a vagrant kubernetes cluster running like this (from the root of the repo):

```
$ cd multi-node/vagrant
$ vagrant up
$ export KUBECONFIG="$(pwd)/kubeconfig"
$ kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):

```
$ kubectl cluster-info
```

If you see a url response, you are ready to go.

### Deploy Rook

Rook can be setup and deployed in Kubernetes by simply deploying the rook-operator deployment manifest.

```
$ kubectl create namespace rook
$ kubectl create -f rook-operator.yaml
```

Use `kubectl` to list pods in the rook namespace. You should be able to see the following: 

```
$ kubectl -n rook get pod
NAME                            READY     STATUS    RESTARTS   AGE
mon0                            1/1       Running   0          1m
mon1                            1/1       Running   0          1m
mon2                            1/1       Running   0          1m
osd-n1sm3                       1/1       Running   0          1m
osd-pb0sh                       1/1       Running   0          1m
osd-rth3q                       1/1       Running   0          1m
rgw-1785797224-9xb4r            1/1       Running   0          1m
rgw-1785797224-vbg8d            1/1       Running   0          1m
rook-api-4184191414-l0wmw       1/1       Running   0          1m
rook-operator-349747813-c3dmm   1/1       Running   0          1m
```

### Create Storage class
Before Rook can start provisioning storage, a StorageClass needs to be created. This is used to specify the storage privisioner, parameters, admin secret and other information needed for Kubernetes to interoperate with Rook for provisioning persistent volumes.
Rook already creates a default admin and demo user, whose secrets are already specified in the sample rook-storageclass.yaml. 

However, before we proceed, we need to specify the Ceph monitor IPs. You can find them by running this line (you will need `jq`).

```
$ kubectl -n rook get pod mon0 mon1 mon2 -o json|jq .items[].status.podIP
"10.2.1.68"
"10.2.2.66"
"10.2.0.41"
``` 

Add these IPs into the `monitors` param of the `rook-storageclass.yaml`. Also append the port `6790` to each IPs. They need to be comma-separated.

Create Rook Storage class:

```
$ kubectl create -f rook-storageclass.yaml
```

### Run sample app

Now that rook is running and integrated with Kubernetes, we can create a sample app to verify it. We will create the classic wordpress and mysql apps.
Both these apps will make use of block volumes provisioned by rook.

Start mysql and wordpress:

```
$ kubectl create -f mysql.yaml
$ kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Get the cluster IP of the wordpress app and enter it in your brower:

```
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.
