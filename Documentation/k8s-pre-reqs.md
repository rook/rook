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

Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate with the `system:masters` privilege:
```
-subj "/CN=admin/O=system:masters"
```

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
You can find the instructions on how to install kubeadm in the [`kubeadm` installation page](https://kubernetes.io/docs/getting-started-guides/kubeadm/).

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
