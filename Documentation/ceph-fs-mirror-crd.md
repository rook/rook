---
title: Filesystem Mirror CRD
weight: 3600
indent: true
---

{% include_relative branch.liquid %}

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](quickstart.md)

# Ceph FilesystemMirror CRD

Rook allows creation and updating the fs-mirror daemon through the custom resource definitions (CRDs).
CephFS will support asynchronous replication of snapshots to a remote (different Ceph cluster) CephFS file system via cephfs-mirror tool.
Snapshots are synchronized by mirroring snapshot data followed by creating a snapshot with the same name (for a given directory on the remote file system) as the snapshot being synchronized.
For more information about user management and capabilities see the [Ceph docs](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#cephfs-mirroring).

## Creating daemon

To get you started, here is a simple example of a CRD to deploy an cephfs-mirror daemon.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystemMirror
metadata:
  name: my-fs-mirror
  namespace: rook-ceph
```

## Settings

If any setting is unspecified, a suitable default will be used automatically.

### FilesystemMirror metadata

- `name`: The name that will be used for the Ceph cephfs-mirror daemon.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace.

### FilesystemMirror Settings

- `placement`: The cephfs-mirror pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/cluster.yaml).
- `annotations`: Key value pair list of annotations to add.
- `labels`: Key value pair list of labels to add.
- `resources`: The resource requirements for the cephfs-mirror pods.
- `priorityClassName`: The priority class to set on the cephfs-mirror pods.

## Configuring mirroring peers

In order to configure mirroring peers, please refer to the [CephFilesystem documentation](ceph-filesystem-crd.md#mirroring).
