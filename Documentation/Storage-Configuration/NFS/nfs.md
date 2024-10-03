---
title: NFS Storage Overview
---

NFS storage can be mounted with read/write permission from multiple pods. NFS storage may be
especially useful for leveraging an existing Rook cluster to provide NFS storage for legacy
applications that assume an NFS client connection. Such applications may not have been migrated to
Kubernetes or might not yet support PVCs. Rook NFS storage can provide access to the same network
filesystem storage from within the Kubernetes cluster via PVC while simultaneously providing access
via direct client connection from within or outside of the Kubernetes cluster.

!!! warning
    Simultaneous access to NFS storage from Pods and from external clients complicates NFS user
    ID mapping significantly. Client IDs mapped from external clients will not be the same as the
    IDs associated with the NFS CSI driver, which mount exports for Kubernetes pods.

!!! warning
    Due to a number of Ceph issues and changes, Rook officially only supports Ceph
    v16.2.7 or higher for CephNFS. If you are using an earlier version, upgrade your Ceph version
    following the advice given in Rook's
    [v1.9 NFS docs](https://rook.github.io/docs/rook/latest/CRDs/ceph-nfs-crd/).

!!! note
    CephNFSes support NFSv4.1+ access only. Serving earlier protocols inhibits responsiveness after
    a server restart.


## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main
[quickstart guide](../../Getting-Started/quickstart.md) as well as a
[Ceph filesystem](../Shared-Filesystem-CephFS/filesystem-storage.md) which will act as the backing
storage for NFS.

Many samples reference the CephNFS and CephFilesystem example manifests
[here](https://github.com/rook/rook/blob/master/deploy/examples/nfs.yaml) and
[here](https://github.com/rook/rook/blob/master/deploy/examples/filesystem.yaml).


## Creating an NFS cluster

Create the NFS cluster by specifying the desired settings documented for the
[NFS CRD](../../CRDs/ceph-nfs-crd.md).


## Creating Exports

When a CephNFS is first created, all NFS daemons within the CephNFS cluster will share a
configuration with no exports defined. When creating an export, it is necessary to specify the
CephFilesystem which will act as the backing storage for the NFS export.

RADOS Gateways (RGWs), provided by [CephObjectStores](../Object-Storage-RGW/object-storage.md), can
also be used as backing storage for NFS exports if desired.

### Using the Ceph Dashboard

Exports can be created via the
[Ceph dashboard](https://docs.ceph.com/en/latest/mgr/dashboard/#nfs-ganesha-management) as well. To
enable and use the Ceph dashboard in Rook, see [here](../Monitoring/ceph-dashboard.md).

### Using the Ceph CLI

The Ceph CLI can be used from the Rook toolbox pod to create and manage NFS exports. To do so, first
ensure the necessary Ceph mgr modules are enabled, if necessary, and that the Ceph orchestrator
backend is set to Rook.

#### Enable the Ceph orchestrator (optional)

```console
ceph mgr module enable rook
ceph mgr module enable nfs
ceph orch set backend rook
```

[Ceph's NFS CLI](https://docs.ceph.com/en/latest/mgr/nfs/#export-management) can create NFS exports
that are backed by [CephFS](https://docs.ceph.com/en/latest/cephfs/nfs/) (a CephFilesystem) or
[Ceph Object Gateway](https://docs.ceph.com/en/latest/radosgw/nfs/) (a CephObjectStore).
`cluster_id` or `cluster-name` in the Ceph NFS docs normally refers to the name of the NFS cluster,
which is the CephNFS name in the Rook context.

For creating an NFS export for the CephNFS and CephFilesystem example manifests, the below command
can be used. This creates an export for the `/test` pseudo path.

```console
ceph nfs export create cephfs my-nfs /test myfs
```

The below command will list the current NFS exports for the example CephNFS cluster, which will give
the output shown for the current example.

```console
$ ceph nfs export ls my-nfs
[
  "/test"
]
```

The simple `/test` export's info can be listed as well. Notice from the example that only NFS
protocol v4 via TCP is supported.

```console
$ ceph nfs export info my-nfs /test
{
  "export_id": 1,
  "path": "/",
  "cluster_id": "my-nfs",
  "pseudo": "/test",
  "access_type": "RW",
  "squash": "none",
  "security_label": true,
  "protocols": [
    4
  ],
  "transports": [
    "TCP"
  ],
  "fsal": {
    "name": "CEPH",
    "user_id": "nfs.my-nfs.1",
    "fs_name": "myfs"
  },
  "clients": []
}
```

If you are done managing NFS exports and don't need the Ceph orchestrator module enabled for
anything else, it may be preferable to disable the Rook and NFS mgr modules to free up a small
amount of RAM in the Ceph mgr Pod.

```console
ceph orch set backend ""
ceph mgr module disable rook
```

## Mounting exports

Each CephNFS server has a unique Kubernetes Service. This is because NFS clients can't readily
handle NFS failover. CephNFS services are named with the pattern
`rook-ceph-nfs-<cephnfs-name>-<id>` `<id>` is a unique letter ID (e.g., a, b, c, etc.) for a given
NFS server. For example, `rook-ceph-nfs-my-nfs-a`.

For each NFS client, choose an NFS service to use for the connection. With NFS v4, you can mount an
export by its path using a mount command like below. You can mount all exports at once by omitting
the export path and leaving the directory as just `/`.

```console
mount -t nfs4 -o proto=tcp <nfs-service-address>:/<export-path> <mount-location>
```


## Exposing the NFS server outside of the Kubernetes cluster

Use a LoadBalancer Service to expose an NFS server (and its exports) outside of the Kubernetes
cluster. The Service's endpoint can be used as the NFS service address when
[mounting the export manually](#mounting-exports). We provide an example Service here:
[`deploy/examples/nfs-load-balancer.yaml`](https://github.com/rook/rook/tree/master/deploy/examples).


## NFS Security
Security options for NFS are documented [here](nfs-security.md).


## Ceph CSI NFS provisioner and NFS CSI driver
The NFS CSI provisioner and driver are documented [here](nfs-csi-driver.md)

## Advanced configuration
Advanced NFS configuration is documented [here](nfs-advanced.md)


## Known issues

Known issues are documented on the [NFS CRD page](../../CRDs/ceph-nfs-crd.md#known-issues).
