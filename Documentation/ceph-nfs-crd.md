---
title: NFS CRD
weight: 3100
indent: true
---
{% include_relative branch.liquid %}

# Ceph NFS Server CRD

## Overview
Rook allows exporting NFS shares of a CephFilesystem or CephObjectStore through the CephNFS custom
resource definition. This will spin up a cluster of
[NFS Ganesha](https://github.com/nfs-ganesha/nfs-ganesha) servers that coordinate with one another
via shared RADOS objects. The servers will be configured for NFSv4.1+ access only, as serving
earlier protocols can inhibit responsiveness after a server restart.

> **WARNING**: Due to a number of Ceph issues and changes, Rook officially only supports Ceph
> v16.2.7 or higher for CephNFS. If you are using an earlier version, upgrade your Ceph version
> following the advice given in Rook's
> [v1.8 NFS docs](https://rook.github.io/docs/rook/v1.8/ceph-nfs-crd.html).

## Samples

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: my-nfs
  namespace: rook-ceph
spec:
  # Settings for the NFS server
  server:
    active: 1

    placement:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - nfs-node
      topologySpreadConstraints:
      tolerations:
      - key: nfs-node
        operator: Exists
      podAffinity:
      podAntiAffinity:

    annotations:
      my-annotation: something

    labels:
      my-label: something

    resources:
      limits:
        cpu: "500m"
        memory: "1024Mi"
      requests:
        cpu: "500m"
        memory: "1024Mi"

    priorityClassName:

    logLevel: NIV_INFO
```

## NFS Settings

### Server
The `server` spec sets configuration for Rook-created NFS-Ganesha servers.

* `active`: The number of active NFS servers. Rook supports creating more than one active NFS
  server, but cannot guarantee high availability. For values greater than 1, see the
  [known issue](#serveractive-count-greater-than-1) below.
* `placement`: Kubernetes placement restrictions to apply to NFS server Pod(s). This is similar to
  placement defined for daemons configured by the
  [CephCluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/cluster.yaml).
* `annotations`: Kubernetes annotations to apply to NFS server Pod(s)
* `labels`: Kubernetes labels to apply to NFS server Pod(s)
* `resources`: Kubernetes resource requests and limits to set on NFS server Pod(s)
* `priorityClassName`: Set priority class name for the NFS server Pod(s)
* `logLevel`: The log level that NFS-Ganesha servers should output.</br>
  Default value: NIV_INFO</br>
  Supported values: NIV_NULL | NIV_FATAL | NIV_MAJ | NIV_CRIT | NIV_WARN | NIV_EVENT | NIV_INFO | NIV_DEBUG | NIV_MID_DEBUG | NIV_FULL_DEBUG | NB_LOG_LEVEL


## Creating Exports
When a CephNFS is first created, all NFS daemons within the CephNFS cluster will share a
configuration with no exports defined.

### Using the Ceph Dashboard
Exports can be created via the
[Ceph dashboard](https://docs.ceph.com/en/latest/mgr/dashboard/#nfs-ganesha-management) for Ceph v16
as well. To enable and use the Ceph dashboard in Rook, see [here](ceph-dashboard.md).

### Using the Ceph CLI
The Ceph CLI can be used from the Rook toolbox pod to create and manage NFS exports. To do so, first
ensure the necessary Ceph mgr modules are enabled, if necessary, and that the Ceph orchestrator
backend is set to Rook.

> #### **Enable the Ceph orchestrator if necessary**
> * Required for Ceph v16.2.7 and below
> * Optional for Ceph v16.2.8 and above
> * Must be disabled for Ceph v17.2.0 due to a [Ceph regression](#ceph-v1720)
> ```console
> ceph mgr module enable rook
> ceph mgr module enable nfs
> ceph orch set backend rook
> ```

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
ceph orch set backend ""
ceph mgr module disable rook
```

### Mounting exports
Each CephNFS server has a unique Kubernetes Service. This is because NFS clients can't readily
handle NFS failover. CephNFS services are named with the pattern
`rook-ceph-nfs-<cephnfs-name>-<id>` `<id>` is a unique letter ID (e.g., a, b, c, etc.) for a given
NFS server. For example, `rook-ceph-nfs-my-nfs-a`.

For each NFS client, choose an NFS service to use for the connection. With NFS v4, you can mount an
export by its path using a mount command like below. You can mount all exports at once by omitting
the export path and leaving the directory as just `/`.
```
mount -t nfs4 -o proto=tcp <nfs-service-address>:/<export-path> <mount-location>
```


<<<<<<< HEAD
## Exposing the NFS server outside of the Kubernetes cluster
Use a LoadBalancer Service to expose an NFS server (and its exports) outside of the Kubernetes
cluster. We provide an example here:
[`deploy/examples/nfs-load-balancer.yaml`](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples).
=======
<<<<<<< HEAD
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
=======
## Exposing the NFS server outside of the Kubernetes cluster
Use a LoadBalancer Service to expose an NFS server (and its exports) outside of the Kubernetes
cluster. The Service's endpoint can be used as the NFS service address when
[mounting the export manually](#mounting-exports). We provide an example Service here:
[`deploy/examples/nfs-load-balancer.yaml`](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples).
>>>>>>> fa27b994c (docs: add info about mounting NFS exports externally)
>>>>>>> 9dd8e00c4 (docs: add info about mounting NFS exports externally)


## Scaling the active server count
It is possible to scale the size of the cluster up or down by modifying the `spec.server.active`
field. Scaling the cluster size up can be done at will. Once the new server comes up, clients can be
assigned to it immediately.

The CRD always eliminates the highest index servers first, in reverse order from how they were
started. Scaling down the cluster requires that clients be migrated from servers that will be
eliminated to others. That process is currently a manual one and should be performed before reducing
the size of the cluster.

> **WARNING:** see the [known issue](#serveractive-count-greater-than-1) below about setting this
> value greater than one.


## Known issues
### server.active count greater than 1
* Active-active scale out does not work well with the NFS protocol. If one NFS server in a cluster
  is offline, other servers may block client requests until the offline server returns, which may
  not always happen due to the Kubernetes scheduler.
  * Workaround: It is safest to run only a single NFS server, but we do not limit this if it
    benefits your use case.

### Ceph v17.2.0
* Ceph NFS management with the Rook mgr module enabled has a breaking regression with the Ceph
  Quincy v17.2.0 release.
  * Workaround: Leave Ceph's Rook orchestrator mgr module disabled. If you have enabled it, you must
    disable it using the snippet below from the toolbox.
    ```console
    ceph orch set backend ""
    ceph mgr module disable rook
    ```


## Advanced configuration
All CephNFS daemons are configured using shared RADOS objects stored in a Ceph pool named `.nfs`.
Users can modify the configuration object for each CephNFS cluster if they wish to customize the
configuration.

### Changing configuration of the .nfs pool
By default, Rook creates the `.nfs` pool with Ceph's default configuration. If you wish to change
the configuration of this pool (for example to change its failure domain or replication factor), you
can create a CephBlockPool with the `spec.name` field set to `.nfs`. This pool **must** be
replicated and **cannot** be erasure coded.
[`deploy/examples/nfs.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/nfs.yaml)
contains a sample for reference.

### Adding custom NFS-Ganesha config file changes
The NFS-Ganesha config file format for these objects is documented in the
[NFS-Ganesha project](https://github.com/nfs-ganesha/nfs-ganesha/wiki).

Use Ceph's `rados` tool from the toolbox to interact with the configuration object. The below
command will get you started by dumping the contents of the config object to stdout. The output will
look something like the example shown if you have already created two exports as documented above.
It is best not to modify any of the export objects created by Ceph so as not to cause errors with
Ceph's export management.

```console
rados --pool <pool> --namespace <namespace> get conf-nfs.<cephnfs-name> -
```
```
%url "rados://<pool>/<namespace>/export-1"
%url "rados://<pool>/<namespace>/export-2"
```

`rados ls` and `rados put` are other commands you will want to work with the other shared
configuration objects.

<<<<<<< HEAD
=======
<<<<<<< HEAD
Of note, it is possible to pre-populate the NFS configuration and export objects prior to starting
NFS servers.
=======
>>>>>>> 9dd8e00c4 (docs: add info about mounting NFS exports externally)
Of note, it is possible to pre-populate the NFS configuration and export objects prior to creating
CephNFS server clusters.


## Ceph CSI NFS provisioner and NFS CSI driver
> **EXPERIMENTAL**: this feature is experimental, and we do not guarantee it is bug-free, nor will
> we support upgrades to future versions

In version 1.9.1, Rook is able to deploy the experimental NFS Ceph CSI driver. This requires Ceph
CSI version 3.6.0 or above. We recommend Ceph v16.2.7 or above.

For this section, we will refer to Rook's deployment examples in the
[deploy/examples](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples) directory.

The Ceph CSI NFS provisioner and driver require additional RBAC to operate. Apply the
`deploy/examples/csi/nfs/rbac.yaml` manifest to deploy the additional resources.

Rook will only deploy the Ceph CSI NFS provisioner and driver components when the
`ROOK_CSI_ENABLE_NFS` config is set to `"true"` in the `rook-ceph-operator-config` configmap. Change
the value in your manifest, or patch the resource as below.
```sh
kubectl --namespace rook-ceph patch configmap rook-ceph-operator-config --type merge --patch '{"data":{"ROOK_CSI_ENABLE_NFS": "true"}}'
```

> **NOTE:** The rook-ceph operator Helm chart will deploy the required RBAC and enable the driver
> components if `csi.nfs.enabled` is set to `true`.

In order to create NFS exports via the CSI driver, you must first create a CephFilesystem to serve
as the underlying storage for the exports, and you must create a CephNFS to run an NFS server that
will expose the exports.

From the examples, `filesystem.yaml` creates a CephFilesystem called `myfs`, and `nfs.yaml` creates
an NFS server called `my-nfs`.

You may need to enable or disable the Ceph orchestrator. Follow the same steps documented
[above](#enable-the-ceph-orchestrator-if-necessary) based on your Ceph version and desires.

You must also create a storage class. Ceph CSI is designed to support any arbitrary Ceph cluster,
but we are focused here only on Ceph clusters deployed by Rook. Let's take a look at a portion of
the example storage class found at `deploy/examples/csi/nfs/storageclass.yaml` and break down how
the values are determined.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-nfs
provisioner: rook-ceph.nfs.csi.ceph.com # [1]
parameters:
  nfsCluster: my-nfs # [2]
  server: rook-ceph-nfs-my-nfs-a # [3]
  clusterID: rook-ceph # [4]
  fsName: myfs # [5]
  pool: myfs-replicated # [6]

  # [7] (entire csi.storage.k8s.io/* section immediately below)
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph

# ... some fields omitted ...
```

1. `provisioner`: **rook-ceph**.nfs.csi.ceph.com because **rook-ceph** is the namespace where the
   CephCluster is installed
2. `nfsCluster`: **my-nfs** because this is the name of the CephNFS
3. `server`: rook-ceph-nfs-**my-nfs**-a because Rook creates this Kubernetes Service for the CephNFS
   named **my-nfs**
4. `clusterID`: **rook-ceph** because this is the namespace where the CephCluster is installed
5. `fsName`: **myfs** because this is the name of the CephFilesystem used to back the NFS exports
6. `pool`: **myfs**-**replicated** because **myfs** is the name of the CephFilesystem defined in
   `fsName` and because **replicated** is the name of a data pool defined in the CephFilesystem
7. `csi.storage.k8s.io/*`: note that these values are shared with the Ceph CSI CephFS provisioner

See `deploy/examples/csi/nfs/pvc.yaml` for an example of how to create a PVC that will create an NFS
<<<<<<< HEAD
export. See `deploy/examples/csi/nfs/pod.yaml` for an example of how this can be connected to an
application pod. Both are quite standard.
=======
export. The export will be created and a PV created for the PVC immediately, even without a Pod to
mount the PVC. The `share` parameter set on the resulting PV contains the share path (`share`) which
can be used as the export path when [mounting the export manually](#mounting-exports).

See `deploy/examples/csi/nfs/pod.yaml` for an example of how a PVC can be connected to an
application pod.
>>>>>>> fa27b994c (docs: add info about mounting NFS exports externally)
>>>>>>> 9dd8e00c4 (docs: add info about mounting NFS exports externally)
