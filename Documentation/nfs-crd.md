---
title: NFS Server CRD
weight: 8000
---

# NFS Server CRD

NFS Server can be created and configured using the `nfsservers.nfs.rook.io` custom resource definition (CRD).
Please refer to the [user guide walk-through](nfs.md) for complete instructions.
This page will explain all the available configuration options on the NFS CRD.

## Sample

The parameters to configure the NFS CRD are demonstrated in the example below which is followed by a table that explains the parameters in more detail.

Below is a very simple example that shows sharing a volume (which could be hostPath, cephFS, cephRBD, googlePD, EBS, etc.) using NFS, without any client or per export based configuration.

For a `PersistentVolumeClaim` named `googlePD-claim`, which has Read/Write permissions and no squashing, the NFS CRD instance would look like the following:

```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: nfs-vol
  namespace: rook
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      accessMode: ReadWrite
      squash: none
    persistentVolumeClaim:
      claimName: googlePD-claim
  # A key/value list of annotations
  annotations:
  #  key: value
```

## Settings

The table below explains in detail each configuration option that is available in the NFS CRD.

| Parameter                                  | Description                                                                                                                                                            | Default     |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- |
| `replicas`                                 | The number of NFS daemon to start                                                                                                                                      | `1`         |
| `annotations`                              | Key value pair list of annotations to add.                                                                                                                             | `[]`        |
| `exports`                                  | Parameters for creating an export                                                                                                                                      | `<empty>`   |
| `exports.name`                             | Name of the volume being shared                                                                                                                                        | `<empty>`   |
| `exports.server`                           | NFS server configuration                                                                                                                                               | `<empty>`   |
| `exports.server.accessMode`                | Volume access modes (Reading and Writing) for the share (Valid options are `ReadOnly`, `ReadWrite` and `none`)                                                         | `ReadWrite` |
| `exports.server.squash`                    | This prevents root users connected remotely from having root privileges (valid options are `none`, `rootId`, `root` and `all`)                                         | `none`      |
| `exports.server.allowedClients`            | Access configuration for clients that can consume the NFS volume                                                                                                       | `<empty>`   |
| `exports.server.allowedClients.name`       | Name of the host/hosts                                                                                                                                                 | `<empty>`   |
| `exports.server.allowedClients.clients`    | The host or network to which the export is being shared. Valid entries for this field are host names, IP addresses, netgroups, and CIDR network addresses.             | `<empty>`   |
| `exports.server.allowedClients.accessMode` | Reading and Writing permissions for the client* (valid options are same as `exports.server.accessMode`)                                                                | `ReadWrite` |
| `exports.server.allowedClients.squash`     | Squash option for the client* (valid options are same as `exports.server.squash`)                                                                                      | `none`      |
| `exports.persistentVolumeClaim`            | The PVC that will serve as the backing volume to be exported by the NFS server. Any PVC is allowed, such as host paths, CephFS, Ceph RBD, Google PD, Amazon EBS, etc.. | `<empty>`   |
| `exports.persistentVolumeClaim.claimName`  | Name of the PVC                                                                                                                                                        | `<empty>`   |

*note: if `exports.server.allowedClients.accessMode` and `exports.server.allowedClients.squash` options are specified, `exports.server.accessMode` and `exports.server.squash` are overridden respectively.

Description for `volumes.allowedClients.squash` valid options are:

| Option   | Description                                                                       |
| -------- | --------------------------------------------------------------------------------- |
| `none`   | No user id squashing is performed                                                 |
| `rootId` | UID `0` and GID `0` are squashed to the anonymous uid and anonymous GID.          |
| `root`   | UID `0` and GID of any value are squashed to the anonymous uid and anonymous GID. |
| `all`    | All users are squashed                                                            |

The volume that needs to be exported by NFS must be attached to NFS server pod via PVC. Examples of volume that can be attached are Host Path, AWS Elastic Block Store, GCE Persistent Disk, CephFS, RBD etc. The limitations of these volumes also apply while they are shared by NFS. The limitation and other details about these volumes can be found [here](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

## Examples

This section contains some examples for more advanced scenarios and configuration options.

### Single volume exported for access by multiple clients

This example shows how to share a volume with different options for different clients accessing the share.
The EBS volume (represented by a PVC) will be exported by the NFS server for client access as `/nfs-share` (note that this PVC must already exist).

The following client groups are allowed to access this share:

* `group1` with IP address `172.17.0.5` will be given Read Only access with the root user squashed.
* `group2` includes both the network range of `172.17.0.5/16` and a host named `serverX`.  They will all be granted Read/Write permissions with no user squash.

```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: nfs-vol
  namespace: rook
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      allowedClients:
      - name: group1
        clients: 172.17.0.5
        accessMode: ReadOnly
        squash: root
      - name: group2
        clients:
        - 172.17.0.0/16
        - serverX
        accessMode: ReadWrite
        squash: none
    persistentVolumeClaim:
      claimName: ebs-claim
```

### Multiple volumes

This section provides an example of how to share multiple volumes from one NFS server.
These volumes can all be different types (e.g., Google PD and Ceph RBD).
Below we will share an Amazon EBS volume as well as a CephFS volume, using differing configuration for the two:

* The EBS volume is named `share1` and is available for all clients with Read Only access and no squash.
* The CephFS volume is named `share2` and is available for all clients with Read/Write access and no squash.

```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: nfs-multi-vol
  namespace: rook
spec:
  replicas: 1
  exports:
  - name: share1
    server:
      allowedClients:
      - name: ebs-host
        clients: all
        accessMode: ReadOnly
        squash: none
    persistentVolumeClaim:
      claimName: ebs-claim
  - name: share2
    server:
      allowedClients:
      - name: ceph-host
        clients: all
        accessMode: ReadWrite
        squash: none
    persistentVolumeClaim:
      claimName: cephfs-claim
```
