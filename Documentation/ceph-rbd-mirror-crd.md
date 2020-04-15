---
title: RBDMirror CRD
weight: 3500
indent: true
---

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
