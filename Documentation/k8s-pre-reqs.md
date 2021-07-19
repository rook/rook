---
title: Prerequisites
weight: 1000
---
{% include_relative branch.liquid %}

# Prerequisites

Rook can be installed on any existing Kubernetes cluster as long as it meets the minimum version
and Rook is granted the required privileges (see below for more information). If you don't have a Kubernetes cluster,
you can quickly set one up using [Minikube](#minikube), [Kubeadm](#kubeadm) or [CoreOS/Vagrant](#new-local-kubernetes-cluster-with-vagrant).

## Minimum Version

Kubernetes **v1.11** or higher is supported for the Ceph operator.
Kubernetes **v1.16** or higher is supported for the Cassandra and NFS operators.

**Important** If you are using K8s 1.15 or older, you will need to create a different version of the Ceph CRDs. Create the `crds.yaml` found in the [pre-k8s-1.16](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/pre-k8s-1.16) subfolder of the example manifests.

## Ceph Prerequisites

See also **[Ceph Prerequisites](ceph-prerequisites.md)**.

## Pod Security Policies

Rook requires privileges to manage the storage in your cluster. If you have Pod Security Policies enabled
please review this section. By default, Kubernetes clusters do not have PSPs enabled so you may
be able to skip this section.

If you are configuring Ceph on OpenShift, the Ceph walkthrough will configure the PSPs as well
when you start the operator with [operator-openshift.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/operator-openshift.yaml).

### Cluster Role

> **NOTE**: Cluster role configuration is only needed when you are not already `cluster-admin` in your Kubernetes cluster!

Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate that is bound to ClusterRole `cluster-admin`.

One simple way to achieve it is to assign your certificate with the `system:masters` group:

```console
-subj "/CN=admin/O=system:masters"
```

`system:masters` is a special group that is bound to `cluster-admin` ClusterRole, but it can't be easily revoked so be careful with taking that route in a production setting.
Binding individual certificate to ClusterRole `cluster-admin` is revocable by deleting the ClusterRoleBinding.

### RBAC for PodSecurityPolicies

If you have activated the [PodSecurityPolicy Admission Controller](https://kubernetes.io/docs/admin/admission-controllers/#podsecuritypolicy) and thus are
using [PodSecurityPolicies](https://kubernetes.io/docs/concepts/policy/pod-security-policy/), you will require additional `(Cluster)RoleBindings`
for the different `ServiceAccounts` Rook uses to start the Rook Storage Pods.

Security policies will differ for different backends. See Ceph's Pod Security Policies set up in
[common.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/common.yaml)
for an example of how this is done in practice.

### PodSecurityPolicy

You need at least one `PodSecurityPolicy` that allows privileged `Pod` execution. Here is an example
which should be more permissive than is needed for any backend:

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: privileged
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
    - '*'
  allowedCapabilities:
    - '*'
  hostPID: true
  # hostNetwork is required for using host networking
  hostNetwork: false
```

**Hint**: Allowing `hostNetwork` usage is required when using `hostNetwork: true` in a Cluster `CustomResourceDefinition`!
You are then also required to allow the usage of `hostPorts` in the `PodSecurityPolicy`. The given
port range will allow all ports:

```yaml
   hostPorts:
     # Ceph msgr2 port
     - min: 1
       max: 65535
```

## Authenticated docker registries

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
$ kubectl -n rook-ceph create secret docker-registry my-registry-secret --docker-server=DOCKER_REGISTRY_SERVER --docker-username=DOCKER_USER --docker-password=DOCKER_PASSWORD --docker-email=DOCKER_EMAIL

# and for namespace rook-ceph (cluster)
$ kubectl -n rook-ceph create secret docker-registry my-registry-secret --docker-server=DOCKER_REGISTRY_SERVER --docker-username=DOCKER_USER --docker-password=DOCKER_PASSWORD --docker-email=DOCKER_EMAIL
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

## Bootstrapping Kubernetes

Rook will run wherever Kubernetes is running. Here are a couple of simple environments to help you get started with Rook.

* [Minikube](https://github.com/kubernetes/minikube/releases): A single-node cluster, simplest to get started
* [Kubeadm](https://kubernetes.io/docs/setup/independent/install-kubeadm/): One or more nodes for more comprehensive deployments
