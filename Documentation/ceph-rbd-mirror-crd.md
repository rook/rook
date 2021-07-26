---
title: RBDMirror CRD
weight: 3500
indent: true
---
{% include_relative branch.liquid %}

# Ceph RBDMirror CRD

Rook allows creation and updating rbd-mirror daemon(s) through the custom resource definitions (CRDs).
RBD images can be asynchronously mirrored between two Ceph clusters.
For more information about user management and capabilities see the [Ceph docs](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/).

## Creating daemons

To get you started, here is a simple example of a CRD to deploy an rbd-mirror daemon.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: my-rbd-mirror
  namespace: rook-ceph
spec:
  count: 1
```

### Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](ceph-quickstart.md)

## Settings

If any setting is unspecified, a suitable default will be used automatically.

### RBDMirror metadata

* `name`: The name that will be used for the Ceph RBD Mirror daemon.
* `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace.

### RBDMirror Settings

* `count`: The number of rbd mirror instance to run.
* `placement`: The rbd mirror pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml).
* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `resources`: The resource requirements for the rbd mirror pods.
* `priorityClassName`: The priority class to set on the rbd mirror pods.

### Configuring mirroring peers

On an external site you want to mirror with, you need to create a bootstrap peer token.
The token will be used by one site to **pull** images from the other site.
The following assumes the name of the pool is "test" and the site name "europe" (just like the region), so we will be pulling images from this site:

```console
external-cluster-console # rbd mirror pool peer bootstrap create test --site-name europe
```

For more details, refer to the official rbd mirror documentation on [how to create a bootstrap peer](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#bootstrap-peers).

When the peer token is available, you need to create a Kubernetes Secret.
Our `europe-cluster-peer-pool-test-1` will have to be created manually, like so:

```console
$ kubectl -n rook-ceph create secret generic "europe-cluster-peer-pool-test-1" \
--from-literal=token=eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ== \
--from-literal=pool=test
```

Rook will read both `token` and `pool` keys of the Data content of the Secret.
Rook also accepts the `destination` key, which specifies the mirroring direction.
It defaults to rx-tx for bidirectional mirroring, but can also be set to rx-only for unidirectional mirroring.

You can now inject the rbdmirror CR:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: my-rbd-mirror
  namespace: rook-ceph
spec:
  count: 1
  peers:
    secretNames:
      - "europe-cluster-peer-pool-test-1"
```

You can add more pools, for this just repeat the above and change the "pool" value of the Kubernetes Secret.
So the list might eventually look like:

```yaml
  peers:
    secretNames:
      - "europe-cluster-peer-pool-test-1"
      - "europe-cluster-peer-pool-test-2"
      - "europe-cluster-peer-pool-test-3"
```

Along with three Kubernetes Secret.
