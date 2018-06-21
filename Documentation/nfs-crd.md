---
title: NFS Export
weight: 38
indent: true
---

# NFS Export CRD
NFS Exports can be created and configured using the `export.nfs.rook.io` custom resource definition (CRD).
Please refer to the [user guide walk-through](nfs.md) for complete instructions.
This page will explain all the available configuration options on the NFS CRD.

## Sample

The parameters to configure NFS CRD are demonstrated in the example bellow which is followed by a table that explains the parameters:

A simple example for sharing a volume(could be hostPath, cephFS, cephRBD, googlePD, EBS etc.) using NFS, without client specification and per export based configuration, whose NFS-Ganesha export entry looks like:
```
EXPORT {
    Export_Id = 1;
    Path = /export;
    Pseudo = /nfs-share;
    Protocols = 4;
    Sectype = sys;
    Access_Type = RW;
    Squash = none;
    FSAL {
        Name = VFS;
    }
}
```  
the CRD instance will look like the following:
```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSExport
metadata:
  name: nfs-vol
  namespace: rook
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      accessMode: ReadWrire
      squash: root
    persistentVolumeClaim:
      claimName: googlePD-claim
```
The table explains each parameter

| Parameter                                 | Description                              | Default                       |
|-------------------------------------------|------------------------------------------|-------------------------------|
| `replicas`                                | The no. of NFS daemon to start           | `1`                           |
| `exports`                                 | Parameters for creating an export        | <none>                        |
| `exports.name`                            | Name of the volume being shared          | <none>                        |
| `exports.server`                          | NFS server configuration                 | <none>                        |
| `exports.server.accessMode`               | Volume access modes(Reading and Writing) for the share          | `ReadOnly` |
| `exports.server.squash`                   | This prevents root users connected remotely from having root privileges  | `root` |
| `exports.server.allowedClients`           | Access configuration for clients that can consume the NFS volume         | <none> |
| `exports.server.allowedClients.name`      | Name of the host/hosts                                                   | <none> |
| `exports.server.allowedClients.clients`   | The host or network to which export is being shared.(could be hostname, ip address, netgroup, CIDR network address, or all) | <none> |
| `exports.server.allowedClients.accessMode` | Reading and Writing permissions for the client*                         | `ReadOnly` |
| `exports.server.allowedClients.squash`    | Squash option for the client*                                          | `root`     |
| `exports.persistentVolumeClaim`      | Claim to get volume(Volume could come from hostPath, cephFS, cephRBD, googlePD, EBS etc. and these volumes will be exposed by NFS server ). | <none> |
| `exports.persistentVolumeClaim.claimName` | Name of the PVC                                         | <none>    |

*note: if `exports.server.accessMode` and `exports.server.squash` options are mentioned, `exports.server.allowedClients.accessMode` and `exports.server.allowedClients.squash` are overridden respectively.

Available options for `volumes.allowedClients.accessMode` are:
1. ReadOnly
2. ReadWrite
3. none

Available options for `volumes.allowedClients.squash` are:
1. none     (No user id squashing is performed)
2. rootId   (uid 0 and gid 0 are squashed to the anonymous uid and anonymous gid)
3. root     (uid 0 and gid of any value are squashed to the anonymous uid and anonymous gid)
4. all      (All users are squashed)

The volume that needs to be exported by NFS must be attached to NFS server pod via PVC. Examples of volume that can be attached are Host Path, AWS Elastic Block Store, GCE Persistent Disk, CephFS, RBD etc. The limitations of these volumes also apply while they are shared by NFS. The limitation and other details about these volumes can be found [here](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

### Examples

Here are some examples for advanced configuration:

1. For sharing a volume(could be hostPath, cephFS, cephRBD, googlePD, EBS etc.) using NFS, which will be shared as /nfs-share by the NFS server with different options for different clients whose NFS-Ganesha export entry looks like:
```
EXPORT {
    Export_Id = 1;
    Path = /export;
    Pseudo = /nfs-share;
    Protocols = 4;
    Sectype = sys;
    FSAL {
        Name = VFS;
    }
    CLIENT {
        Clients = 172.17.0.5;
        Access_Type = RO;
        Squash = root;
    }
    CLIENT {
        Clients = 172.17.0.0/16, node-1;
        Access_Type = RW;
        Squash = none;
    }
}
```  
the CRD instance will look like the following:
```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSExport
metadata:
  name: nfs-vol
  namespace: rook
spec:
  replicas: 1
  exports:
  - name: nfs-share
    server:
      allowedClients:
      - name: host1
        clients: 172.17.0.5
        accessMode: ReadOnly
        squash: root
      - name: host2
        clients:
        - 172.17.0.0/16
        - serverX
        accessMode: ReadWrire
        squash: none
    persistentVolumeClaim:
      claimName: ebs-claim
```

2. For sharing multiple volumes using NFS, which will be shared as /share1 and /share2 by the NFS server whose NFS-Ganesha export entry looks like:
```
EXPORT {
    Export_Id = 1;
    Path = /export;
    Pseudo = /share1;
    Protocols = 4;
    Sectype = sys;
    FSAL {
        Name = VFS;
    }
    CLIENT {
        Clients = all;
        Access_Type = RO;
        Squash = none;
    }
}
EXPORT {
    Export_Id = 2;
    Path = /export2;
    Pseudo = /share2;
    Protocols = 4;
    Sectype = sys;
    FSAL {
        Name = VFS;
    }
    CLIENT {
        Clients = all;
        Access_Type = RW;
        Squash = none;
    }
}
```  
the CRD instance will look like the following:
```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSExport
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