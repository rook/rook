---
title: CephBlockPoolRados Namespace CRD
---

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](../../Getting-Started/quickstart.md)

RADOS currently uses pools both for data distribution (pools are shared into
PGs, which map to OSDs) and as the granularity for security (capabilities can
restrict access by pool).  Overloading pools for both purposes makes it hard to
do multi-tenancy because it not a good idea to have a very large number of
pools.

A namespace would be a division of a pool into separate logical namespaces. For
more information about BlockPool and namespace refer to the [Ceph
docs](https://docs.ceph.com/en/latest/man/8/rbd/)

Having multiple namespaces in a pool would allow multiple Kubernetes clusters
to share one unique ceph cluster without creating a pool per kubernetes cluster
and it will also allow to have tenant isolation between multiple tenants in a
single Kubernetes cluster without creating multiple pools for tenants.

Rook allows creation of Ceph BlockPool
[RadosNamespaces](https://docs.ceph.com/en/latest/man/8/rbd/) through the
custom resource definitions (CRDs).

## Example

To get you started, here is a simple example of a CR to create a CephBlockPoolRadosNamespace on the CephBlockPool "replicapool".

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPoolRadosNamespace
metadata:
  name: namespace-a
  namespace: rook-ceph # namespace:cluster
spec:
  # The name of the CephBlockPool CR where the namespace is created.
  blockPoolName: replicapool
```

## Settings

If any setting is unspecified, a suitable default will be used automatically.

### Metadata

- `name`: The name that will be used for the Ceph BlockPool rados namespace.

### Spec

- `blockPoolName`: The metadata name of the CephBlockPool CR where the rados namespace will be created.
