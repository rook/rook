---
title: Prerequisites
weight: 11
indent: true
---

# Prerequisites

Rook can be installed on any existing Kubernetes clusters as long as it meets the minimum version and have the required priviledge to run in the cluster (see below for more information). If you dont have a Kubernetes cluster, you can quickly set one up using [Minikube](#minikube), [Kubeadm](#kubeadm) or [CoreOS/Vagrant](#new-local-kubernetes-cluster-with-vagrant).

## Minimum Version

Kubernetes v1.6 or higher is targeted by Rook (while Rook is in alpha it will track the latest release to use the latest features).

Support is available for Kubernetes v1.5.2, although your mileage may vary.
You will need to use the yaml files from the [1.5 folder](/cluster/examples/kubernetes/1.5).

## Privileges

Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate that is bound to ClusterRole `cluster-admin`.

One simple way to achieve it is to assign your certificate with the `system:masters` group:
```
-subj "/CN=admin/O=system:masters"
```

`system:masters` is a special group that is bound to `cluster-admin` ClusterRole, but it can't be easily revoked so be careful with taking that route in a production setting.
Binding individual certificate to ClusterRole `cluster-admin` is revocable by deleting the ClusterRoleBinding.

## Flexvolume Configuration

Rook uses [Flexvolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) to integrate with Kubernetes for performing storage operations. In some operating systems where Kubernetes is deployed, the [default Flexvolume plugin directory](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#prerequisites) (the directory where flexvolume drivers are installed) is **read-only**.

This is the case for Kubernetes deployments on CoreOS and Rancher.
In these environments, the Kubelet needs to be told to use a different flexvolume plugin directory that is accessible and writeable.
To do this, you will need to first add the `--volume-plugin-dir` flag to the Kubelet and then restart the Kubelet process. 
These steps need to be carried out on **all nodes** in your cluster.

### CoreOS Container Linux

In CoreOS, our recomendation is to specify the flag as shown below:

```bash
--volume-plugin-dir=/var/lib/kubelet/volumeplugins
```

Restart Kubelet in order for this change to take effect.

### Rancher

Rancher provides an easy way to configure Kubelet. This flag can be provided to the Kubelet configuration template at deployment time or by using the `up to date` feature if Kubernetes is already deployed.

To configure Flexvolume in Rancher, specify this Kubelet flag as shown below:

```bash
--volume-plugin-dir=/var/lib/kubelet/volumeplugins
```

Restart Kubelet in order for this change to take effect.

## Minikube

To install `minikube`, refer to this [page](https://github.com/kubernetes/minikube/releases). Once you have `minikube` installed, start a cluster by doing the following:

```console
$ minikube start
Starting local Kubernetes cluster...
Starting VM...
SSH-ing files into VM...
Setting up certs...
Starting cluster components...
Connecting to cluster...
Setting up kubeconfig...
Kubectl is now configured to use the cluster.
```

After these steps, your minikube cluster is ready to install Rook on.

## Kubeadm

You can easily spin up Rook on top of a `kubeadm` cluster.
You can find the instructions on how to install kubeadm in the [Install `kubeadm`] (https://kubernetes.io/docs/setup/independent/install-kubeadm/) page.

By using `kubeadm`, you can use Rook in just a few minutes!

## New local Kubernetes cluster with Vagrant

For a quick start with a new local cluster, use the Rook fork of [coreos-kubernetes](https://github.com/rook/coreos-kubernetes). This will bring up a multi-node Kubernetes cluster with `vagrant` and CoreOS virtual machines ready to use Rook immediately.

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


## Using Rook in Kubernetes

Now that you have a Kubernetes cluster running, you can start using Rook with [these steps](kubernetes.md#deploy-rook).
