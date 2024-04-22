---
title: CephClient CRD
---

Rook allows creation and updating clients through the custom resource definitions (CRDs).
For more information about user management and capabilities see the [Ceph docs](https://docs.ceph.com/docs/master/rados/operations/user-management/).

## Use Case: Connecting to Ceph

Use Client CRD in case you want to integrate Rook with applications that are using LibRBD directly.
For example for OpenStack deployment with Ceph backend use Client CRD to create OpenStack services users.

The Client CRD is not needed for Flex or CSI driver users. The drivers create the needed users automatically.

### Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Quickstart guide](../Getting-Started/quickstart.md).

### 1. Creating Ceph User

To get you started, here is a simple example of a CRD to configure a Ceph client with capabilities.

```yaml
---
apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: example
  namespace: rook-ceph
spec:
  caps:
    mon: 'profile rbd, allow r'
    osd: 'profile rbd pool=volumes, profile rbd pool=vms, profile rbd-read-only pool=images'
```

To use `CephClient` to connect to a Ceph cluster:

### 2. Find the generated secret for the `CephClient`

Once your `CephClient` has been processed by Rook, it will be updated to include your secret:

```console
kubectl -n rook-ceph get cephclient example -o jsonpath='{.status.info.secretName}'
```

### 3. Extract Ceph cluster credentials from the generated secret

Extract Ceph cluster credentials from the generated secret (note that the subkey will be your original client name):

```console
kubectl --namespace rook-ceph get secret rook-ceph-client-example -o jsonpath="{.data.example}" | base64 -d
```

The base64 encoded value that is returned **is** the password for your ceph client.

### 4. Retrieve the mon endpoints

To send writes to the cluster, you must retrieve the mons in use:

```console
kubectl --namespace rook-ceph get configmap rook-ceph-mon-endpoints -o jsonpath='{.data.data}' | sed 's/.=//g'`
```

This command should produce a line that looks somewhat like this:

```console
10.107.72.122:6789,10.103.244.218:6789,10.99.33.227:6789
```

### 5. (optional) Generate Ceph configuration files

If you choose to generate files for Ceph to use you will need to generate the following files:

- General configuration file (ex. `ceph.conf`)
- Keyring file (ex. `ceph.keyring`)

Examples of the files follow:

`ceph.conf`

```ini
[global]
mon_host=10.107.72.122:6789,10.103.244.218:6789,10.99.33.227:6789
log file = /tmp/ceph-$pid.log
```

`ceph.keyring`

```ini
[client.example]
  key = < key, decoded from k8s secret>
  # The caps below are for a rbd workload -- you may need to edit/modify these capabilities for other workloads
  # see https://docs.ceph.com/en/latest/cephfs/capabilities
  caps mon = 'allow r'
  caps osd = 'profile rbd pool=<your pool>, profile rb pool=<another pool>'
```

### 6. Connect to the Ceph cluster with your given client ID

With the files we've created, you should be able to query the cluster by setting Ceph ENV variables and running `ceph status`:

```console
export CEPH_CONF=/libsqliteceph/ceph.conf;
export CEPH_KEYRING=/libsqliteceph/ceph.keyring;
export CEPH_ARGS=--id example;
ceph status
```

With this config, the ceph tools (`ceph` CLI, in-program access, etc) can connect to and utilize the Ceph cluster.

## Use Case: SQLite

The Ceph project contains a [SQLite VFS][sqlite-vfs] that interacts with RADOS directly, called [`libcephsqlite`][libcephsqlite].

First, on your workload ensure that you have the appropriate packages installed that make `libcephsqlite.so` available:

- [`ceph` on Alpine](https://pkgs.alpinelinux.org/package/edge/community/x86_64/ceph)
- [`libsqlite3-mod-ceph` on Ubuntu](https://pkgs.alpinelinux.org/package/edge/community/x86_64/ceph)
- [`libcephsqlite` on Fedora](https://pkgs.org/search/?q=libcephsqlite)
- [`ceph` on CentOS](https://cbs.centos.org/koji/packageinfo?packageID=534)

Without the appropriate package (or a from-scratch build of SQLite), you will be unable to load `libcephsqlite.so`.

After creating a `CephClient` similar to [`deploy/examples/sqlitevfs-client.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/sqlitevfs-client.yaml) and retrieving it's credentials, you may set the following ENV variables:

```console
export CEPH_CONF=/libsqliteceph/ceph.conf;
export CEPH_KEYRING=/libsqliteceph/ceph.keyring;
export CEPH_ARGS=--id sqlitevfs
```

Then start your SQLite database:

```console
sqlite> .load libcephsqlite.so
sqlite> .open file:///poolname:/test.db?vfs=ceph
sqlite>
```

If those lines complete without error, you have successfully set up SQLite to access Ceph.

See [the libcephsqlite documentation][libcephsqlite] for more information on the VFS and database URL format.

[libcephsqlite]: https://docs.ceph.com/en/latest/rados/api/libcephsqlite/
[sqlite-vfs]: https://www.sqlite.org/vfs.html
