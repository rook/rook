---
title: Prerequisites
weight: 1000
---

# Prerequisites

Rook can be installed on any existing Kubernetes cluster as long as it meets the minimum version
and Rook is granted the required privileges (see below for more information). If you don't have a Kubernetes cluster,
you can quickly set one up using [Minikube](#minikube), [Kubeadm](#kubeadm) or [CoreOS/Vagrant](#new-local-kubernetes-cluster-with-vagrant).

## Minimum Version

Kubernetes v1.13 or higher is supported by Rook.

## Privileges and RBAC

Rook requires privileges to manage the storage in your cluster. See the details [here](psp.md) for
setting up Rook in a Kubernetes cluster with Pod Security Policies enabled.

## Flexvolume Configuration

The Rook agent requires setup as a Flex volume plugin to manage the storage attachments in your cluster.
See the [Flex Volume Configuration](flexvolume.md) topic to configure your Kubernetes deployment to load the Rook volume plugin.

## Kernel

### RBD

Rook Ceph requires a Linux kernel built with the RBD module. Many distributions of Linux have this module but some don't,
e.g. the GKE Container-Optimised OS (COS) does not have RBD. You can test your Kubernetes nodes by running `modprobe rbd`.
If it says 'not found', you may have to [rebuild your kernel](https://rook.io/docs/rook/master/common-issues.html#rook-agent-rbd-module-missing-error)
or choose a different Linux distribution.

### CephFS

If you will be creating volumes from a Ceph shared file system (CephFS), the recommended minimum kernel version is 4.17.
If you have a kernel version less than 4.17, the requested PVC sizes will not be enforced. Storage quotas will only be
enforced on newer kernels.

## Kernel modules directory configuration

Normally, on Linux, kernel modules can be found in `/lib/modules`. However, there are some distributions that put them elsewhere. In that case the environment variable `LIB_MODULES_DIR_PATH` can be used to override the default. Also see the documentation in [helm-operator](helm-operator.md) on the parameter `agent.libModulesDirPath`. One notable distribution where this setting is useful would be [NixOS](https://nixos.org).

## Extra agent mounts

On certain distributions it may be necessary to mount additional directories into the agent container. That is what the environment variable `AGENT_MOUNTS` is for. Also see the documentation in [helm-operator](helm-operator.md) on the parameter `agent.mounts`. The format of the variable content should be `mountname1=/host/path1:/container/path1,mountname2=/host/path2:/container/path2`.

## LVM package

Some Linux distributions do not ship with the `lvm2` package. This package is required on all storage nodes in your k8s cluster. Please install it using your Linux distribution's package manager; for example:

```console
# Centos
sudo yum install -y lvm2

# Ubuntu
sudo apt-get install -y lvm2
```

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

```console
git clone https://github.com/rook/coreos-kubernetes.git
cd coreos-kubernetes/multi-node/vagrant
vagrant up
export KUBECONFIG="$(pwd)/kubeconfig"
kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):

```console
kubectl cluster-info
```

Once you see a url response, your cluster is [ready for use by Rook](ceph-quickstart.md#deploy-rook).

## Support for authenticated docker registries

If you want to use an image from authenticated docker registry (e.g. for image cache/mirror), you'll need to
add an `imagePullSecret` to all relevant service accounts. This way all pods created by the operator (for service account:
`rook-ceph-system`) or all new pods in the namespace (for service account: `default`) will have the `imagePullSecret` added
to their spec.

The whole process is described in the [official kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account).

### Example setup for a ceph cluster

To get you started, here's a quick rundown for the ceph example from the [quickstart guide](/Documentation/ceph-quickstart.md).

First, we'll create the secret for our registry as described [here](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod):

```console
# for namespace rook-ceph
kubectl -n rook-ceph create secret docker-registry my-registry-secret --docker-server=DOCKER_REGISTRY_SERVER --docker-username=DOCKER_USER --docker-password=DOCKER_PASSWORD --docker-email=DOCKER_EMAIL

# and for namespace rook-ceph (cluster)
kubectl -n rook-ceph create secret docker-registry my-registry-secret --docker-server=DOCKER_REGISTRY_SERVER --docker-username=DOCKER_USER --docker-password=DOCKER_PASSWORD --docker-email=DOCKER_EMAIL
```

Next we'll add the following snippet to all relevant service accounts as described [here](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account):

```yaml
imagePullSecrets:
- name: my-registry-secret
```

The service accounts are:

* `rook-ceph-system` (namespace: `rook-ceph`): Will affect all pods created by the rook operator in the `rook-ceph` namespace.
* `default` (namespace: `rook-ceph`): Will affect most pods in the `rook-ceph` namespace.
* `rook-ceph-mgr` (namespace: `rook-ceph`): Will affect the MGR pods in the `rook-ceph` namespace.
* `rook-ceph-osd` (namespace: `rook-ceph`): Will affect the OSD pods in the `rook-ceph` namespace.

You can do it either via e.g. `kubectl -n <namespace> edit serviceaccount default` or by modifying the [`operator.yaml`](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml)
and [`cluster.yaml`](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/cluster.yaml) before deploying them.

Since it's the same procedure for all service accounts, here is just one example:

```console
kubectl -n rook-ceph edit serviceaccount default
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: default
  namespace: rook-ceph
secrets:
- name: default-token-12345
imagePullSecrets:                # here are the new
- name: my-registry-secret       # parts
```

After doing this for all service accounts all pods should be able to pull the image from your registry.

## Using Rook in Kubernetes

Now that you have a Kubernetes cluster running, you can start using Rook with [these steps](ceph-quickstart.md#deploy-rook).

## Using Rook on Tectonic Bare Metal

Follow [these instructions](tectonic.md) to run Rook on Tectonic Kubernetes
