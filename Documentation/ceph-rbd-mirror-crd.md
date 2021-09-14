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

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](quickstart.md)

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

Configure mirroring peers individually for each CephBlockPool. Refer to the
[CephBlockPool documentation](ceph-pool-crd.md#mirroring) for more detail.
