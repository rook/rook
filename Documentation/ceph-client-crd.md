---
title: Client CRD
weight: 3500
indent: true
---

# Ceph Client CRD

Rook allows creation and updating clients through the custom resource definitions (CRDs).
For more information about user management and capabilities see the [Ceph docs](https://docs.ceph.com/docs/master/rados/operations/user-management/).

## Use Case

Use Client CRD in case you want to integrate Rook with with applications that are using LibRBD directly.
For example for OpenStack deployment with Ceph backend use Client CRD to create OpenStack services users.

The Client CRD is not needed for Flex or CSI driver users. The drivers create the needed users automatically.

## Creating Ceph User

To get you started, here is a simple example of a CRD to configure a Ceph client with capabilities.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: glance
  namespace: rook-ceph
spec:
  caps:
    mon: 'profile rbd'
    osd: 'profile rbd pool=images'
---
apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: cinder
  namespace: rook-ceph
spec:
  caps:
    mon: 'profile rbd'
    osd: 'profile rbd pool=volumes, profile rbd pool=vms, profile rbd-read-only pool=images'
```

### Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](quickstart.md)
