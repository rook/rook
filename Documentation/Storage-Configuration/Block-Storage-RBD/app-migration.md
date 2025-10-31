---
title: Application Migration
---

When there are multiple Kubernetes clusters that are configured to connect
to the same external Ceph cluster, the applications running in each K8s cluster
will be storing the data in the same central Ceph cluster.

If one application cluster is becoming overloaded or goes down, applications
can be migrated to another cluster.

<insert pic>

### Defining Migratable Applications

Configuring an application to be migratable between clusters requires the following:

- One external Ceph cluster that is accessible from the K8s application clusters.
  The Ceph cluster could be configured by Rook with [host networking](../../CRDs/Cluster/network-providers.md#host-networking),
  or configured with some other tools such as cephadm.
- The application clusters must configure Rook to connect to the same
  [external cluster](../../CRDs/Cluster/external-cluster/external-cluster.md),
  with access to the same Ceph pool.
- For each application
    - Create an RBD image in the external Ceph pool
    - Create a static PV
    - Create a PVC that binds to the static PV

1. On the external Ceph cluster, create an RBD image with the command:

```console
rbd create <name> --size <size> --pool <poolName>
```

Parameters:
- `name`: The name of the rbd image
- `size`: The size of the rbd image (with suffix M/G/T)
- `poolName`: The name of the Ceph pool where the rbd image is stored

For example:

```console
rbd create static-image-abc --size 100G --pool replicapool
```

2. On the application cluster where the application is to be started,
   create the application's PVC and the static PV.
   See the example [pvc-static.yaml](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/pvc-static.yaml).

Take note of the settings:
- The PVC defines an empty `storageClassName`
- The name of the rbd image created in the previous step must match the PV `volumeHandle`
- The pool name in the PV must match the pool where the rbd image was created
- The `clusterID` of the PV is the namespace where Rook is running
- The `csi.driver` prefix is the namespace where Rook is running
- `persistentVolumeReclaimPolicy` must be `Retain`

### Migrating an Application

When the application needs to be migrated to another cluster:

1. Scale down the application in the first cluster.
2. Ensure the application is stopped and is not capable of accidentally connecting
   to Ceph. RBD volumes have no protection against corruption from multiple instances
   of an application accessing the same volume across clusters.
3. Create the PVC and static PV in the new cluster.
4. Create or scale up the application in the new cluster.

### RWX Applications

Application migration can also be configured for CephFS RXW volumes.
This would allow multiple instances of the application to run
simultaneously across clusters.

The configuration for CephFS RWX volumes is similar to the RBD RWO
static PV definition. See the
[Ceph-CSI Static PVC](https://github.com/ceph/ceph-csi/blob/04981f8b75b53e0dcf89a31625368eba3bbe9439/docs/static-pvc.md#cephfs-static-pvc)
topic for an example of using CephFS.
