---
title: Authenticated Container Registries
---

If you want to use an image from authenticated docker registry (e.g. for image cache/mirror), you'll need to
add `imagePullSecrets` to all relevant service accounts. See the next section for the required service accounts.

The whole process is described in the [official kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account).

## Example setup for a ceph cluster

To get you started, here's a quick rundown for the ceph example from the [quickstart guide](../quickstart.md).

First, we'll create the secret for our registry as described [here](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod) (the secret will be created in the `rook-ceph` namespace, make sure to change it if your Rook Ceph Operator/Cluster is in another namespace):

```console
kubectl -n rook-ceph create secret docker-registry my-registry-secret --docker-server=DOCKER_REGISTRY_SERVER --docker-username=DOCKER_USER --docker-password=DOCKER_PASSWORD --docker-email=DOCKER_EMAIL
```

Next we'll add the following snippet to all relevant service accounts as described [here](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account):

```yaml
imagePullSecrets:
- name: my-registry-secret
```

The service accounts are:

* `rook-ceph-system` (namespace: `rook-ceph`): Will affect all pods created by the rook operator in the `rook-ceph` namespace.
* `rook-ceph-default` (namespace: `rook-ceph`): Will affect most pods in the `rook-ceph` namespace.
* `rook-ceph-mgr` (namespace: `rook-ceph`): Will affect the MGR pods in the `rook-ceph` namespace.
* `rook-ceph-osd` (namespace: `rook-ceph`): Will affect the OSD pods in the `rook-ceph` namespace.
* `rook-ceph-rgw` (namespace: `rook-ceph`): Will affect the RGW pods in the `rook-ceph` namespace.

Since it's the same procedure for all service accounts, here is just one example:

```console
kubectl -n rook-ceph edit serviceaccount rook-ceph-default
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-default
  namespace: rook-ceph
secrets:
- name: default-token-12345
# Add the highlighted lines:
imagePullSecrets:
- name: my-registry-secret
```

After doing this for all service accounts all pods should be able to pull the image from your registry.
