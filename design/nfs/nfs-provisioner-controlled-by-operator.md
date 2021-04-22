# NFS Provisioner Controlled by Operator

## Summary

NFS Provisioner is a built in dynamic provisioner for Rook NFS. The functionality works fine but has an issue where the provisioner uses the same underlying directory for each provisioned PV when provisioning two or more PV in the same share/export. This overlap means that each provisioned PV for a share/export can read/write each others data.

This hierarchy is the current behaviour of NFS Provisioner when provisioning two PV in the same share/export:

```text
export
├── sample-export
|   ├── data (from PV-A)
|   ├── data (from PV-B)
|   ├── data (from PV-A)
|   └── data (from PV-A)
└── another-export
```

Both PV-A and PV-B uses the `sample-export` directory as their data location.

This proposal is to make Rook NFS Provisioner create a sub-directory for every provisioned PV in the same share/export. So it will have a hierarchy like:

```text
export
├── sample-export
│   ├── pv-a
│   │   ├── data (from PV-A)
│   │   ├── data (from PV-A)
│   │   └── data (from PV-A)
│   └── pv-b
│       └── data (from PV-B)
└── another-export
```

Since those directories are not in the NFS Provisioner pod but in the NFS Server pod, NFS Provisioner cannot directly create sub-directories for them. The solution is to mount the whole underlying NFS share/export directory so that the NFS Provisioner can create a sub-directory for each provisioned PV.

### Original Issue

- https://github.com/rook/rook/issues/4982

### Goals

- Make NFS Provisioner to create sub-directory for each provisioned PVs.
- Make NFS Provisioner use the sub-directory for each provisioned PV instead of using underlying directory.
- Improve reliability of NFS Provisioner.

### Non-Goals

- NFS Operator manipulates uncontrolled resources.

## Proposal details

The approach will be similar to [Kubernetes NFS Client Provisioner](https://github.com/kubernetes-incubator/external-storage/tree/master/nfs-client), where the provisioner mounts the whole of NFS share/export into the provisioner pod (by kubelet), so that the provisioner can then create the appropriate sub-directory for each provisioned PV. Currently Rook NFS Provisioner is deployed independently and before the NFS Server itself, so we cannot mount the NFS share because we don't know the NFS Server IP or the share/export directory.

The idea is to make NFS Provisioner controlled by the operator. So when an NFS Server is created, the operator also then creates its provisioner, which mounts each NFS share/export. Then, the NFS Provisioner can create a sub-directory for each provisioned PV.

This is the example NFS Server

```yaml
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: rook-nfs
  namespace: rook-nfs
spec:
  replicas: 1
  exports:
  - name: share1
    ...
    persistentVolumeClaim:
      claimName: nfs-default-claim
  - name: share2
    ...
    persistentVolumeClaim:
      claimName: nfs-another-claim
```

And the operator will creates the provisioner deployment like

```yaml
kind: Deployment
apiVersion: apps/v1
metadata:
  name: rook-nfs-provisioner
  namespace: rook-nfs
spec:
  ...
    spec:
      ....
      containers:
      - name: rook-nfs-provisioner
        image: rook/nfs:v1.6.1
        args: ["nfs", "provisioner","--provisioner=nfs.rook.io/nfs-server-provisioner"]
      volumes:
        - name: share1
          nfs:
            server: <NFS_SERVER_IP>
            path: /export/nfs-default-claim
        - name: share2
          nfs:
            server: <NFS_SERVER_IP>
            path: /export/nfs-another-claim
```

The provisioner deployment will be created in the same namespace as the NFS server and with the same privileges. Since the provisioner is automatically created by the operator, the provisioner deployment name and provisioner name flag (`--provisioner`) value will depend on NFSServer name. The provisioner deployment name will have an added suffix of `-provisioner` and the provisioner name will start with `nfs.rook.io/`.

## Alternatives

The other possible approach is NFS Provisioner mounts the NFS Server share manually (by executing `mount` command) before creating an appropriate directory for each PV. But in my humble opinion, NFS Provisioner would be lacking reliability under several conditions like NFSServer getting its exports updated, the cluster has two or more NFSServer, etc.

## Glossary

**Provisioned PV:** Persistent Volumes which provisioned by rook nfs provisioner through Storage Class and Persistent Volumes Claims.

**NFS share/export:** A directory in NFS Server which exported using nfs protocol.
