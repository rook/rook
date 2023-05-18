---
title: Advanced configuration
---

All CephNFS daemons are configured using shared RADOS objects stored in a Ceph pool named `.nfs`.
Users can modify the configuration object for each CephNFS cluster if they wish to customize the
configuration.

## Changing configuration of the .nfs pool

By default, Rook creates the `.nfs` pool with Ceph's default configuration. If you wish to change
the configuration of this pool (for example to change its failure domain or replication factor), you
can create a CephBlockPool with the `spec.name` field set to `.nfs`. This pool **must** be
replicated and **cannot** be erasure coded.
[`deploy/examples/nfs.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/nfs.yaml)
contains a sample for reference.

## Adding custom NFS-Ganesha config file changes

Ceph uses [NFS-Ganesha](https://github.com/nfs-ganesha/nfs-ganesha) servers. The config file format
for these objects is documented in the
[NFS-Ganesha project](https://github.com/nfs-ganesha/nfs-ganesha/wiki).

Use Ceph's `rados` tool from the toolbox to interact with the configuration object. The below
command will get you started by dumping the contents of the config object to stdout. The output will
look something like the example shown if you have already created two exports as documented above.
It is best not to modify any of the export objects created by Ceph so as not to cause errors with
Ceph's export management.

```console
$ rados --pool <pool> --namespace <namespace> get conf-nfs.<cephnfs-name> -
%url "rados://<pool>/<namespace>/export-1"
%url "rados://<pool>/<namespace>/export-2"
```

`rados ls` and `rados put` are other commands you will want to work with the other shared
configuration objects.

Of note, it is possible to pre-populate the NFS configuration and export objects prior to creating
CephNFS server clusters.

## Creating NFS export over RGW
!!! warning
    RGW NFS export is experimental for the moment. It is not recommended for scenario of modifying existing content.

For creating an NFS export over RGW(CephObjectStore) storage backend, the below command
can be used. This creates an export for the `/testrgw` pseudo path on an existing bucket bkt4exp as an example. You could use `/testrgw` pseudo for nfs mount operation afterwards.

```console
ceph nfs export create rgw my-nfs /testrgw bkt4exp
```
