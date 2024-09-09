---
title: CephBlockPoolRados Namespace CRD
---

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](../../Getting-Started/quickstart.md)

RADOS currently uses pools both for data distribution (pools are shared into
PGs, which map to OSDs) and as the granularity for security (capabilities can
restrict access by pool).  Overloading pools for both purposes makes it hard to
do multi-tenancy because it is not a good idea to have a very large number of
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

- `mirroring`: Sets up mirroring of the rados namespace (requires Ceph v20 or newer)
    - `mode`: mirroring mode to run, possible values are "pool" or "image" (required). Refer to the [mirroring modes Ceph documentation](https://docs.ceph.com/docs/master/rbd/rbd-mirroring/#enable-mirroring) for more details
    - `remoteNamespace`: Name of the rados namespace on the peer cluster where the namespace should get mirrored. The default is the same rados namespace.
    - `snapshotSchedules`: schedule(s) snapshot at the **rados namespace** level. It is an array and one or more schedules are supported.
        - `interval`: frequency of the snapshots. The interval can be specified in days, hours, or minutes using d, h, m suffix respectively.
        - `startTime`: optional, determines at what time the snapshot process starts, specified using the ISO 8601 time format.

## Creating a Storage Class

Once the RADOS namespace is created, an RBD-based StorageClass can be created to
create PVs in this RADOS namespace. For this purpose, the `clusterID` value from the
CephBlockPoolRadosNamespace status needs to be put into the `clusterID` field of the StorageClass
spec.

Extract the clusterID from the CephBlockPoolRadosNamespace CR:

```console
$ kubectl -n rook-ceph  get cephblockpoolradosnamespace/namespace-a -o jsonpath='{.status.info.clusterID}'
80fc4f4bacc064be641633e6ed25ba7e
```

In this example, replace `namespace-a` by the actual name of the radosnamespace
created before.
Now set the `clusterID` retrieved from the previous step into the `clusterID` of the storage class.

Example:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block-rados-ns
provisioner: rook-ceph.rbd.csi.ceph.com # csi-provisioner-name
parameters:
  clusterID: 80fc4f4bacc064be641633e6ed25ba7e
  pool: replicapool
  ...
```

### Mirroring

First, enable mirroring for the parent CephBlockPool.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  replicated:
    size: 3
  mirroring:
    enabled: true
    mode: image
    # schedule(s) of snapshot
    snapshotSchedules:
      - interval: 24h # daily snapshots
        startTime: 14:00:00-05:00
```

Second, configure the rados namespace CRD with the mirroring:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPoolRadosNamespace
metadata:
  name: namespace-a
  namespace: rook-ceph # namespace:cluster
spec:
  # The name of the CephBlockPool CR where the namespace is created.
  blockPoolName: replicapool
  mirroring:
    mode: image
    remoteNamespace: namespace-a # default is the same as the local rados namespace
    # schedule(s) of snapshot
    snapshotSchedules:
      - interval: 24h # daily snapshots
        startTime: 14:00:00-05:00
```
