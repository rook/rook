---
title: FilesystemMirror CRD
weight: 3600
indent: true
---
{% include_relative branch.liquid %}

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](ceph-quickstart.md)

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


## Configuring mirroring peers

On an external site you want to mirror with, you need to create a bootstrap peer token.
The token will be used by one site to **pull** images from the other site.
The following assumes the name of the pool is "test" and the site name "europe" (just like the region), so we will be pulling images from this site:

```console
external-cluster-console # ceph fs snapshot mirror peer_bootstrap create myfs2 client.mirror europe
{"token": "eyJmc2lkIjogIjgyYjdlZDkyLTczYjAtNGIyMi1hOGI3LWVkOTQ4M2UyODc1NiIsICJmaWxlc3lzdGVtIjogIm15ZnMyIiwgInVzZXIiOiAiY2xpZW50Lm1pcnJvciIsICJzaXRlX25hbWUiOiAidGVzdCIsICJrZXkiOiAiQVFEVVAxSmdqM3RYQVJBQWs1cEU4cDI1ZUhld2lQK0ZXRm9uOVE9PSIsICJtb25faG9zdCI6ICJbdjI6MTAuOTYuMTQyLjIxMzozMzAwLHYxOjEwLjk2LjE0Mi4yMTM6Njc4OV0sW3YyOjEwLjk2LjIxNy4yMDc6MzMwMCx2MToxMC45Ni4yMTcuMjA3OjY3ODldLFt2MjoxMC45OS4xMC4xNTc6MzMwMCx2MToxMC45OS4xMC4xNTc6Njc4OV0ifQ=="}
```

For more details, refer to the official ceph-fs mirror documentation on [how to create a bootstrap peer](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#bootstrap-peers).

When the peer token is available, you need to create a Kubernetes Secret, it can named anything.
Our `europe-cluster-peer-fs-test-1` will have to be created manually, like so:

```console
$ kubectl -n rook-ceph create secret generic "europe-cluster-peer-fs-test-1" \
--from-literal=token=eyJmc2lkIjogIjgyYjdlZDkyLTczYjAtNGIyMi1hOGI3LWVkOTQ4M2UyODc1NiIsICJmaWxlc3lzdGVtIjogIm15ZnMyIiwgInVzZXIiOiAiY2xpZW50Lm1pcnJvciIsICJzaXRlX25hbWUiOiAidGVzdCIsICJrZXkiOiAiQVFEVVAxSmdqM3RYQVJBQWs1cEU4cDI1ZUhld2lQK0ZXRm9uOVE9PSIsICJtb25faG9zdCI6ICJbdjI6MTAuOTYuMTQyLjIxMzozMzAwLHYxOjEwLjk2LjE0Mi4yMTM6Njc4OV0sW3YyOjEwLjk2LjIxNy4yMDc6MzMwMCx2MToxMC45Ni4yMTcuMjA3OjY3ODldLFt2MjoxMC45OS4xMC4xNTc6MzMwMCx2MToxMC45OS4xMC4xNTc6Njc4OV0ifQ==
```

Rook will read a `token` key of the Data content of the Secret.

You can now create the mirroring CR:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystemMirror
metadata:
  name: my-fs-mirror
  namespace: rook-ceph
spec:
  peers:
    secretNames:
      - "europe-cluster-peer-pool-test-1"
```

You can add more filesystems by repeating the above and changing the "token" value of the Kubernetes Secret.
So the list might eventually look like:

```yaml
  peers:
    secretNames:
      - "europe-cluster-peer-fs-test-1"
      - "europe-cluster-peer-fs-test-2"
      - "europe-cluster-peer-fs-test-3"
```

Along with three Kubernetes Secret.


## Settings

If any setting is unspecified, a suitable default will be used automatically.

### FilesystemMirror metadata

* `name`: The name that will be used for the Ceph cephfs-mirror daemon.
* `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace.

### FilesystemMirror Settings

* `peers`: to configure mirroring peers
  * `secretNames`:  a list of peers to connect to. Currently (Ceph Pacific release) **only a single** peer is supported where a peer represents a Ceph cluster.
  However, if you want to enable mirroring of multiple filesystems, you would have to have **one Secret per filesystem**.
* `placement`: The cephfs-mirror pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml).
* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `resources`: The resource requirements for the cephfs-mirror pods.
* `priorityClassName`: The priority class to set on the cephfs-mirror pods.

