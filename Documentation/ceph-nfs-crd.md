---
title: NFS CRD
weight: 3100
indent: true
---

# Ceph NFS Server CRD

## Overview
Rook allows exporting NFS shares of a CephFilesystem or CephObjectStore through the CephNFS custom
resource definition. This will spin up a cluster of
[NFS Ganesha](https://github.com/nfs-ganesha/nfs-ganesha) servers that coordinate with one another
via shared RADOS objects. The servers will be configured for NFSv4.1+ access only, as serving
earlier protocols can inhibit responsiveness after a server restart.

> **WARNING**: We do not recommend using NFS in Ceph v16.2.0 through v16.2.6. If you are using Ceph
> v15, we encourage you to upgrade directly to Ceph Pacific v16.2.7.
> [Upgrade steps are outlined below.](#upgrading-from-ceph-v15-to-v16)

## Samples
The following sample assumes Ceph v16 and will create a two-node active-active cluster of NFS
Ganesha gateways.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: my-nfs
  namespace: rook-ceph
spec:
  # For Ceph v15, the rados block is required. It is ignored for Ceph v16.
  rados:
    # RADOS pool where NFS configs are stored.
    # In this example the data pool for the "myfs" filesystem is used.
    # If using the object store example, the data pool would be "my-store.rgw.buckets.data".
    # Note that this has nothing to do with where exported file systems or object stores live.
    pool: myfs-data0
    # RADOS namespace where NFS client recovery data is stored in the pool.
    namespace: nfs-ns

  # Settings for the NFS server
  server:
    # the number of active NFS servers
    active: 2
    # A key/value list of annotations
    annotations:
    #  key: value
    # where to run the NFS server
    placement:
    #  nodeAffinity:
    #    requiredDuringSchedulingIgnoredDuringExecution:
    #      nodeSelectorTerms:
    #      - matchExpressions:
    #        - key: role
    #          operator: In
    #          values:
    #          - mds-node
    #  tolerations:
    #  - key: mds-node
    #    operator: Exists
    #  podAffinity:
    #  podAntiAffinity:
    #  topologySpreadConstraints:

    # The requests and limits set here allow the ganesha pod(s) to use half of one CPU core and 1 gigabyte of memory
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
    # the priority class to set to influence the scheduler's pod preemption
    priorityClassName:
```

## NFS Settings

### RADOS Settings
NFS configuration is stored in a Ceph pool so that it is highly available and protected. How that is
configured changes depending on the Ceph version. Configuring the pool is done via the `rados` config.

> **WARNING**: Do not use [erasure coded (EC) pools](ceph-pool-crd.md#erasure-coded) for NFS.
> NFS-Ganesha uses OMAP which is not supported by Ceph's erasure coding.

#### For Ceph v16 or newer
* `poolConfig`: (optional) The pool settings to use for the RADOS pool.
  It matches the [CephBlockPool](ceph-block.md) specification.
  The settings will be applied to a pool named `.nfs` on Ceph v16.2.7 or newer.

#### For Ceph v15
* `pool`: (mandatory) The Ceph pool where NFS configuration is stored.
* `namespace`: (mandatory) The namespace in the `pool` where configuration objects will be stored.

Rook ignores both `pool` and `namespace` ([see above](#for-ceph-v15)) settings when running Ceph v16
or newer.


## Creating Exports
When a CephNFS is first created, all NFS daemons within the CephNFS cluster will share a
configuration with no exports defined.

### For Ceph v16 or newer
For Ceph v16.2.0 through v16.2.6, exports cannot be managed through the Ceph dashboard, and
newly-created Ceph command line tools are lacking. We highly recommend using Ceph v16.2.7 or higher
with NFS, which fixes bugs and streamlines export management, allowing exports to be created via the
Ceph Dashboard and the Ceph CLI. With v16.2.7 or higher, the Ceph dashboard and Ceph CLI will be
able to manage the same NFS exports interchangeably as desired.

#### Using the Ceph Dashboard
Exports can be created via the
[Ceph dashboard](https://docs.ceph.com/en/latest/mgr/dashboard/#nfs-ganesha-management) for Ceph v16
as well. To enable and use the Ceph dashboard in Rook, see [here](ceph-dashboard.md).

#### Using the Ceph CLI
The Ceph CLI can be used from the Rook toolbox pod to create and manage NFS exports. To do so, first
ensure the necessary Ceph mgr modules are enabled and that the Ceph orchestrator backend is set to
Rook.
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
ceph nfs export ls my-nfs
```
```
[
  "/test"
]
```

The simple `/test` export's info can be listed as well. Notice from the example that only NFS
protocol v4 via TCP is supported.
```console
ceph nfs export info my-nfs /test
```
```
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
ceph mgr module disable nfs
ceph mgr module disable rook
```

### Mounting exports
Each CephNFS server has a unique Kubernetes Service. This is because NFS clients can't readily
handle NFS failover. CephNFS services are named with the pattern
`rook-ceph-nfs-<cephnfs-name>-<id>` `<id>` is a unique letter ID (e.g., a, b, c, etc.) for a given
NFS server. For example, `rook-ceph-nfs-my-nfs-a`.

For each NFS client, choose an NFS service to use for the connection. With NFS v4, you can mount all
exports at once to a mount location.
```
mount -t nfs4 -o proto=tcp <nfs-service-ip>:/ <mount-location>
```

### For Ceph v15
Exports can be created via the
[Ceph dashboard](https://docs.ceph.com/en/octopus/mgr/dashboard/#dashboard-nfs-ganesha-management)
for Ceph v15. To enable and use the Ceph dashboard in Rook, see [here](ceph-dashboard.md).

Enable the creation of NFS exports in the dashboard for a given cephfs or object gateway pool by
running the following command in the toolbox container:
* [For a single CephNFS cluster](https://docs.ceph.com/en/octopus/mgr/dashboard/#configuring-nfs-ganesha-in-the-dashboard)
  ```console
  ceph dashboard set-ganesha-clusters-rados-pool-namespace <pool>[/<namespace>]
  ```
* [For multiple CephNFS clusters](https://docs.ceph.com/en/octopus/mgr/dashboard/#support-for-multiple-nfs-ganesha-clusters)
  ```console
  ceph dashboard set-ganesha-clusters-rados-pool-namespace <cephnfs-name>:<pool>[/<namespace>](,<cephnfs-name>:<pool>[/<namespace>])*
  ```
  For each of the multiple entries above, `cephnfs-name` is the name given to CephNFS resource by the
  manifest's `metadata.name`: `my-nfs` for the example earlier in this document. `pool` and
  `namespace` are the same configured via the CephNFS spec's `rados` block.

You should now be able to create exports from the
[Ceph dashboard](https://docs.ceph.com/en/octopus/mgr/dashboard/#ceph-dashboard).

**You may need to enable exports created by the dashboard before they will work!**

Creating exports via the dashboard does not necessarily enable them. Newer versions of Ceph v15
enable the exports automatically, but not all. To ensure the exports are created automatically, use
Ceph v15.2.15 or higher. Otherwise, you must take the manual steps below to ensure the exports are
enabled.

To enable exports, we are going to modify the Ceph RADOS object (stored in Ceph) that defines the
configuration shared by all NFS daemons.

Please note that `<pool>` and `<namespace>` will continue to refer to the configured `rados` spec's
`pool` and `namespace` for a particular CephNFS cluster.

List the shared configuration objects in a Ceph pool with this command from the Ceph toolbox.
```console
rados --pool <pool> --namespace <namespace> ls
```

The output may look something like below after you have created two exports. Here we have used the
`my-nfs` example CephNFS.
```
conf-nfs.my-nfs
export-1
export-2
grace
rec-0000000000000002:my-nfs.a
```

The configuration of NFS daemons, and enabling exports, is controlled by the `conf-nfs.my-nfs`
object in this example. The object name follows the `conf-nfs.<cephnfs-name>` pattern.

Get the contents of the config file, which may be empty as in this example.
```console
rados --pool <pool> --namespace <namespace> get conf-nfs.my-nfs my-nfs.conf
cat my-nfs.conf
```

Modify the `my-nfs.conf` file above to add URLs for enabling exports.
```
%url "rados://<pool>/<namespace>/export-1"
%url "rados://<pool>/<namespace>/export-2"
```

Then write the modified file to the RADOS config object.
```console
rados --pool <pool> --namespace <namespace> put conf-nfs.my-nfs my-nfs.conf
```

Verify the changes are saved by getting the config again, just as before.
```console
rados --pool <pool> --namespace <namespace> get conf-nfs.my-nfs my-nfs.conf
cat my-nfs.conf
```
```
%url "rados://<pool>/<namespace>/export-1"
%url "rados://<pool>/<namespace>/export-2"
```


## Upgrading from Ceph v15 to v16
We do not recommend using NFS in Ceph v16.2.0 through v16.2.6 due to bugs in Ceph's NFS
implementation. If you are using Ceph v15, we encourage you to upgrade directly to Ceph v16.2.7.

### Prep
To upgrade, first follow the [usual Ceph upgrade steps](ceph-upgrade.md#ceph-version-upgrades). When
the upgrade completes, this will result in NFS exports that no longer work. The dashboard's NFS
management will also be broken. We must now migrate the NFS exports to Ceph's new management method.

We will do all work from the toolbox pod. Exec into an interactive session there.

First, unset the previous dashboard configuration with the below command.
```sh
ceph dashboard set-ganesha-clusters-rados-pool-namespace ""
```

Also ensure the necessary Ceph mgr modules are enabled and that the Ceph orchestrator backend is set
to Rook.
```console
ceph mgr module enable rook
ceph mgr module enable nfs
ceph orch set backend rook
```

### Step 1
Pick a CephNFS to work with and make a note of the `spec.rados.pool` and `spec.rados.namespace`. If
the `pool` is not set, it is `.nfs`. We will refer to these as pool/`<pool>` and
namespace/`<namespace>` for the remainder of the steps. Also note the name of the CephNFS resource,
which will be referred to as CephNFS name or `<cephnfs-name>`.

### Step 2
List the exports defined in the pool.
```sh
rados --pool <pool> --namespace <namespace> ls
```

This may look something like below.
```
grace
rec-0000000000000002:my-nfs.a
export-1
export-2
conf-nfs.my-nfs
```

### Step 3
For each export above, save the export to an `<export>.conf` file.
```
EXPORT="export-1" # "export-2", "export-3", etc.
rados --pool <pool> --namespace <namespace> get "$EXPORT" "/tmp/$EXPORT.conf"
```

The file should contain content similar to what is shown here.
```
$ cat /tmp/export-1.conf
EXPORT {
    export_id = 1;
    path = "/";
    pseudo = "/test";
    access_type = "RW";
    squash = "no_root_squash";
    protocols = 4;
    transports = "TCP";
    FSAL {
        name = "CEPH";
        user_id = "admin";
        filesystem = "myfs";
        secret_access_key = "AQAyr69hwddJERAAE9WdFCmY10fqehzK3kabFw==";
    }

}
```

### Step 4
We will now import each export into Ceph's new format. Perform this step for each export you wish to
migrate.

First remove the `FSAL` configuration block's `user_id` and `secret_access_key` configuration items.
It is sufficient to delete the lines in the `/tmp/<export>.conf` file using `vi` or some other
editor. The file should look similar to below when the edit is finished.
```
$ cat /tmp/<export>.conf
EXPORT {
    export_id = 1;
    path = "/";
    pseudo = "/test";
    access_type = "RW";
    squash = "no_root_squash";
    protocols = 4;
    transports = "TCP";
    FSAL {
        name = "CEPH";
        filesystem = "myfs";
    }

}
```

Now that the old user and access key are removed, import the export. There should be no errors, but
if there are, follow the error message instructions to proceed.
```sh
ceph nfs export apply <cephnfs-name> -i /tmp/<export>.conf
```

### Step 5
Once all exports have been migrated for the current CephNFS, it is good to verify the exports. Use
`ceph nfs export ls <cephnfs-name>` to list all exports (identified by the pseudo path), and use
`ceph nfs export info <cephnfs-name> <export-pseudo>` to inspect the configuration. An export
configuration may look something like below. The [v16 CLI section above](#using-the-ceph-cli) shows
this in more detail.

### Step 6
Repeat these [steps](#step-1) for each other CephNFS.

Clean up all `<export>.conf` files before moving onto subsequent CephNFSes to avoid confusion.
```
rm -f /tmp/export-*.conf
```

### Wrap-up
Once you are finished migrating all CephNFSes, the migration is complete. If you wish to use the
Ceph dashboard to manage exports, you should now be able to find them all listed there.

If you are done managing NFS exports via the CLI and don't need the Ceph orchestrator module enabled
for anything else, it may be preferable to disable the Rook and NFS mgr modules to free up a small
amount of RAM in the Ceph mgr Pod.
```console
ceph mgr module disable nfs
ceph mgr module disable rook
```


## Scaling the active server count
It is possible to scale the size of the cluster up or down by modifying the `spec.server.active`
field. Scaling the cluster size up can be done at will. Once the new server comes up, clients can be
assigned to it immediately.

The CRD always eliminates the highest index servers first, in reverse order from how they were
started. Scaling down the cluster requires that clients be migrated from servers that will be
eliminated to others. That process is currently a manual one and should be performed before reducing
the size of the cluster.


## Advanced configuration
All CephNFS daemons are configured using shared configuration objects stored in Ceph. In general,
users should only need to modify the configuration object. Exports can be created via the simpler
Ceph-provided means documented above.

For configuration and advanced usage, the format for these objects is documented in the
[NFS Ganesha](https://github.com/nfs-ganesha/nfs-ganesha/wiki) project.

Use Ceph's `rados` tool from the toolbox to interact with the configuration object. The below
command will get you started by dumping the contents of the config object to stdout. The output may
look something like the example shown.
```console
rados --pool <pool> --namespace <namespace> get conf-nfs.<cephnfs-name> -
```
```
%url "rados://<pool>/<namespace>/export-1"
%url "rados://<pool>/<namespace>/export-2"
```

`rados ls` and `rados put` are other commands you will want to work with the other shared
configuration objects.

Of note, it is possible to pre-populate the NFS configuration and export objects prior to starting
NFS servers.
