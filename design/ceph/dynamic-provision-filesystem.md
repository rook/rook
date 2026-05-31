# File System Dynamic Provisioning

## Overview

Currently in Rook, to consume a CephFS, the user will have to specify the CephFS volume plugin as well as the required inputs. Some of this inputs are very cumbersome and required hacky commands to obtain them. We should use the dynamic provision feature with PVCs and PVs to facilitate the consumption of CephFS. There are many benefits to this.
Using PVCs allows us to deeply adhere to the Kubernetes API. That means, we get all the features that Kubernetes gives us like: setting reclaim policy on the volume, use RBAC on provisioning and defining accessmode.
On consumption, the pod only has to reference the PVCs. That means, the pod manifest doesn't have to change whether you change the PVC to use block or filesystems.

Another benefit is that it allows us to consume StorageClass that the admin users define and create. The users don't have to worry about metadataPool, erasureCoded, affinity, toleration etc. All they care is creating a filesystem PVC and referencing a storageClass that matches their filesystem needs.

This feature has already been asked by a few users in our community. An issue has been created https://github.com/rook/rook/issues/1125.
Also Dynamic Provision Filesystem is a concept that has already been done in https://github.com/kubernetes-incubator/external-storage/tree/master/ceph/cephfs. So Rook can adopt a similar approach.

## Current Experience

To consume a filesystem, the experience is the following:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
spec:
  strategy:
    type: Recreate
  template:
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: MYSQL_ROOT_PASSWORD
          value: changeme
        ports:
        - containerPort: 3306
          name: mysql
        volumeMounts:
        - name: mysql-persistent-storage
          mountPath: /var/lib/mysql
      volumes:
      - name: mysql-persistent-storage
        cephfs:
          monitors:
          - monitor1
          - monitor2
          - monitor3
          user: admin
          secretRef:
            name: rook-admin
```

Users will have to come up with these values and ensure every parameter is provided correctly.

## Experience with Dynamic Provisioned Filesystem

To create a filesystem, you just create a PVC object. This is consistent with all other storage provisioning in Kubernetes.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: myfsdata
spec:
  storageClassName: rook-filesystem-simple
  path: /myData # Will use root path, "/", if not provided
  accessModes:
  - ReadWriteMany
```

To consume it, the pod manifest is shown as follows:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
spec:
  strategy:
    type: Recreate
  template:
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: MYSQL_ROOT_PASSWORD
          value: changeme
        ports:
        - containerPort: 3306
          name: mysql
        volumeMounts:
        - name: mysql-persistent-storage
          mountPath: /var/lib/mysql
      volumes:
      - name: mysql-persistent-storage
        persistentVolumeClaim:
          claimName: myfsdata
```

Note that the consuming pod manifest looks the same whether it is mounting a filesystem or a block device.

## StorageClass Example

Notice that there was a reference to a StorageClass called `rook-filesystem-simple` in the filesystem PVC example I previously showed. Dynamic provisioned storage refers to a StorageClass object for details and configuration about how the storage should be provisioned.
The storage class is setup by the administrator and can look as follows for filesystem:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-filesystem-simple
provisioner: rook.io/filesystem
parameters:
  fsName: myFS # Name of the filesystem to use.
```

The referenced filesystem, `myFS`, would have to be also created by the admin using a [Filesystem CRD](/Documentation//CRDs/Shared-Filesystem/ceph-filesystem-crd.md).

The admin could also have created a more detailed StorageClass for more a durable filesystem as follows. Lets call it `rook-filesystem-gold`:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-filesystem-gold
provisioner: rook.io/filesystem
parameters:
  fsName: mySuperHAFS
```

With multiple storage class objects, users can refer to many filesystems that match their needs.

## Implementation

In order to do this, we will need to leverage the external-provisioner controller to watch for PVC objects. The external-provisioner controller is already being used by Rook for provisioning block devices.

The implementation logic will look similar to the logic done for block devices. The provisioner will watch for PVC objects of types `rook.io/filesystem`. When the PVC is created, the provisioner will parse for the filesystem information from the StorageClass and create a volume source with all required information. Similarly, when the PVC is deleted, the underlying filesystem components (mds, data pools, etc) will also be deleted.
