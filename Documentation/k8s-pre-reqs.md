
## New Local Kubernetes Cluster
For a quick start with a new local cluster, use the Rook fork of [coreos-kubernetes](https://github.com/rook/coreos-kubernetes). This will bring up a multi-node Kubernetes cluster with `vagrant` and CoreOS virtual machines ready to use `rook` immediately.
```
git clone https://github.com/rook/coreos-kubernetes.git
cd coreos-kubernetes/multi-node/vagrant
vagrant up
export KUBECONFIG="$(pwd)/kubeconfig"
kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):

```
kubectl cluster-info
```

Once you see a url response, your cluster is [ready for use by Rook](kubernetes.md#deploy-rook).

## Minikube
If using `minikube`, you can deploy Rook to it with a small update, which modifies the minikube host to install the `rbd` command. This is needed by the Kubernetes `rbd` volume plugin. To install `minikube`, refer to this [page](https://github.com/kubernetes/minikube/releases).

Once you have `minikube` installed, start a cluster by doing the following:
```
$ minikube start --iso-url=https://s3-us-west-1.amazonaws.com/minikube-rook/minikube.iso
Starting local Kubernetes cluster...
Starting VM...
SSH-ing files into VM...
Setting up certs...
Starting cluster components...
Connecting to cluster...
Setting up kubeconfig...
Kubectl is now configured to use the cluster.
```

SSH into the minikube host and install `rbd`:
```
$ minikube ssh
$ cd /bin
$ sudo curl -O https://raw.githubusercontent.com/ceph/ceph-docker/master/examples/kubernetes-coreos/rbd
$ sudo chmod +x /bin/rbd
$ rbd #run command to download ceph images.
Unable to find image 'ceph/base:latest' locally
latest: Pulling from ceph/base
...
$ exit
```

After these steps, your minikube cluster is ready to install rook on.

## Existing Kubernetes Cluster
Alternatively, if you already have a running Kubernetes cluster, you can deploy Rook to it with a small update to modify the kubelet service to bind mount `/sbin/modprobe`, which allows access to `modprobe`.
Access to modprobe is necessary for using the rbd volume plugin, which is being tracked in the Kubernetes code base with [#23924](https://github.com/kubernetes/kubernetes/issues/23924).  

If using RKT, you can enable `modprobe` by following this [guide](https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md#allow-pods-to-use-rbd-volumes). Instructions have been directly copied below for your convenience:  

Add the following options to the `RKT_OPTS` env before launching the kubelet via kubelet-wrapper:
```ini
[Service]
Environment=KUBELET_VERSION=v1.5.3_coreos.0
Environment="RKT_OPTS=--volume modprobe,kind=host,source=/usr/sbin/modprobe \
  --mount volume=modprobe,target=/usr/sbin/modprobe \
  --volume lib-modules,kind=host,source=/lib/modules \
  --mount volume=lib-modules,target=/lib/modules \
  --uuid-file-save=/var/run/kubelet-pod.uuid"
...
```

Note that the kubelet also requires access to the userspace `rbd` tool that is included only in hyperkube images tagged `v1.3.6_coreos.0` or later. See next section on how to deploy `rbd`.

### Ceph and RBD utilities installed on the nodes

The Kubernetes kubelet shells out to system utilities to mount Rook volumes. This means that every Kubernetes host must have these utilities installed. This requirement extends to the control plane, since there may be interactions between kube-controller-manager and the Ceph cluster. Login to each Kubernetes host where Kubelet runs and execute the following:

For Debian-based distros:

```
apt-get install ceph-fs-common ceph-common
```

For Redhat-based distros:

```
yum install ceph
```

For other Linux distros that don't have an explicit package manager, such as CoreOS, you can use a container with the ceph utilities. To deploy the containers on your hosts, do the following:

```
cd /bin
sudo curl -O https://raw.githubusercontent.com/ceph/ceph-docker/master/examples/kubernetes-coreos/rbd
sudo chmod +x /bin/rbd
rbd #Command to download ceph images.
```

## Kubernetes with RBAC
RBAC restricts what operations can be performed in the cluster. In particular, the operator will be denied access to create third party resources (TPRs) if RBAC is enabled. These steps will give permissions to the `Rook` operator. 

Find the name of the admin role for the cluster.
```
kubectl get clusterrolebinding
```
The role may be `cluster-admin`, or simply `admin` depending on your deployment of Kubernetes.

Now replace the admin name in the `roleRef` section of [rook-rbac.yaml](/demo/kubernetes/rook-rbac.yaml) if it is different this sample of `cluster-admin`.

```
apiVersion: rbac.authorization.k8s.io/v1alpha1
kind: ClusterRoleBinding
metadata:
  name: rook-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: default
  namespace: default
```

Create the cluster role binding. The Rook operator should be good to go.
```
kubectl create -f rook-rbac.yaml
```

## Using Rook in Kubernetes
Now that you have a Kubernetes cluster running, you can start using `rook` with [these steps](kubernetes.md#deploy-rook).