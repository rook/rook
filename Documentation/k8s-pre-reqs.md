---
title: Prerequisites
weight: 10
---

# Prerequisites

Rook can be installed on any existing Kubernetes clusters as long as it meets the minimum version and have the required privilege to run in the cluster (see below for more information). If you dont have a Kubernetes cluster, you can quickly set one up using [Minikube](#minikube), [Kubeadm](#kubeadm) or [CoreOS/Vagrant](#new-local-kubernetes-cluster-with-vagrant).

## Minimum Version

Kubernetes v1.7 or higher is supported by Rook.

## Privileges and RBAC

Rook requires privileges to manage the storage in your cluster. See the details [here](rbac.md) for setting up RBAC.

## Flexvolume Configuration

The Rook agent requires setup as a Flex volume plugin to manage the storage attachments in your cluster.
See the [Flex Volume Configuration](flexvolume.md) topic to configure your Kubernetes deployment to load the Rook volume plugin.

## Bootstrapping Kubernetes

Rook will run wherever Kubernetes is running. Here are some simple environments to help you get started with Rook.

### Minikube

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

### Kubeadm

You can easily spin up Rook on top of a `kubeadm` cluster.
You can find the instructions on how to install kubeadm in the [Install `kubeadm`](https://kubernetes.io/docs/setup/independent/install-kubeadm/) page.

By using `kubeadm`, you can use Rook in just a few minutes!

### New local Kubernetes cluster with Vagrant

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

Once you see a url response, your cluster is [ready for use by Rook](ceph-quickstart.md#deploy-rook).


## Using Rook in Kubernetes

Now that you have a Kubernetes cluster running, you can start using Rook with [these steps](ceph-quickstart.md#deploy-rook).

## Using Rook on Tectonic Bare Metal

Follow [these instructions](tectonic.md) to run Rook on Tectonic Kubernetes
