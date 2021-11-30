---
title: Authenticated Registries
weight: 1100
indent: true
---

## Authenticated docker registries

If you want to use an image from authenticated docker registry (e.g. for image cache/mirror), you'll need to
add an `imagePullSecret` to all relevant service accounts. This way all pods created by the operator (for service account:
`rook-ceph-system`) or all new pods in the namespace (for service account: `default`) will have the `imagePullSecret` added
to their spec.

The whole process is described in the [official kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account).

### Example setup for a ceph cluster

To get you started, here's a quick rundown for the ceph example from the [quickstart guide](/Documentation/quickstart.md).

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

You can do it either via e.g. `kubectl -n <namespace> edit serviceaccount default` or by modifying the [`operator.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/operator.yaml)
and [`cluster.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/cluster.yaml) before deploying them.

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
