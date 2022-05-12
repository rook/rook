---
title: RBD Asynchronous DR Failover and Failback
---

## Planned Migration and Disaster Recovery

Rook comes with the volume replication support, which allows users to perform disaster recovery and planned migration of clusters.

The following document will help to track the procedure for failover and failback in case of a Disaster recovery or Planned migration use cases.

!!! note
    The document assumes that RBD Mirroring is set up between the peer clusters.
    For information on rbd mirroring and how to set it up using rook, please refer to
    the [rbd-mirroring guide](rbd-mirroring.md).

## Planned Migration

!!! info
    Use cases: Datacenter maintenance, technology refresh, disaster avoidance, etc.

### Relocation

The Relocation operation is the process of switching production to a
 backup facility(normally your recovery site) or vice versa. For relocation,
 access to the image on the primary site should be stopped.
The image should now be made *primary* on the secondary cluster so that
 the access can be resumed there.

!!! note
    :memo: Periodic or one-time backup of
    the application should be available for restore on the secondary site (cluster-2).

Follow the below steps for planned migration of workload from the primary
 cluster to the secondary cluster:

* Scale down all the application pods which are using the
 mirrored PVC on the Primary Cluster.
* [Take a backup](rbd-mirroring.md#backup-&-restore) of PVC and PV object from the primary cluster.
 This can be done using some backup tools like
 [velero](https://velero.io/docs/main/).
* [Update VolumeReplication CR](rbd-mirroring.md#create-a-volumereplication-cr) to set `replicationState` to `secondary` at the Primary Site.
 When the operator sees this change, it will pass the information down to the
  driver via GRPC request to mark the dataSource as `secondary`.
* If you are manually recreating the PVC and PV on the secondary cluster,
 remove the `claimRef` section in the PV objects. (See [this](rbd-mirroring.md#restore-the-backup-on-cluster-2) for details)
* Recreate the storageclass, PVC, and PV objects on the secondary site.
* As you are creating the static binding between PVC and PV, a new PV won’t
 be created here, the PVC will get bind to the existing PV.
* [Create the VolumeReplicationClass](rbd-mirroring.md#create-a-volume-replication-class-cr) on the secondary site.
* [Create VolumeReplications](rbd-mirroring.md#create-a-volumereplication-cr) for all the PVC’s for which mirroring
 is enabled
  * `replicationState` should be `primary` for all the PVC’s on
   the secondary site.
* [Check VolumeReplication CR status](rbd-mirroring.md#checking-replication-status) to verify if the image is marked `primary` on the secondary site.
* Once the Image is marked as `primary`, the PVC is now ready
 to be used. Now, we can scale up the applications to use the PVC.

!!! warning
    :memo: In Async Disaster recovery use case, we don't get
    the complete data.
    We will only get the crash-consistent data based on the snapshot interval time.

## Disaster Recovery

!!! info
    Use cases: Natural disasters, Power failures, System failures, and crashes, etc.

!!! note
    To effectively resume operations after a failover/relocation,
    backup of the kubernetes artifacts like deployment, PVC, PV, etc need to be created beforehand by the admin; so that the application can be restored on the peer cluster. For more information, see [backup and restore](rbd-mirroring.md#backup-&-restore).

### Failover (abrupt shutdown)

In case of Disaster recovery, create VolumeReplication CR at the Secondary Site.
 Since the connection to the Primary Site is lost, the operator automatically
 sends a GRPC request down to the driver to forcefully mark the dataSource as `primary`
 on the Secondary Site.

* If you are manually creating the PVC and PV on the secondary cluster, remove
 the claimRef section in the PV objects. (See [this](rbd-mirroring.md#restore-the-backup-on-cluster-2) for details)
* Create the storageclass, PVC, and PV objects on the secondary site.
* As you are creating the static binding between PVC and PV, a new PV won’t be
 created here, the PVC will get bind to the existing PV.
* [Create the VolumeReplicationClass](rbd-mirroring.md#create-a-volume-replication-class-cr) and [VolumeReplication CR](rbd-mirroring.md#create-a-volumereplication-cr) on the secondary site.
* [Check VolumeReplication CR status](rbd-mirroring.md#checking-replication-status) to verify if the image is marked `primary` on the secondary site.
* Once the Image is marked as `primary`, the PVC is now ready to be used. Now,
 we can scale up the applications to use the PVC.

### Failback (post-disaster recovery)

Once the failed cluster is recovered on the primary site and you want to failback
 from secondary site, follow the below steps:

* Scale down the running applications (if any) on the primary site.
 Ensure that all persistent volumes in use by the workload are no
 longer in use on the primary cluster.
* [Update VolumeReplication CR](rbd-mirroring.md#create-a-volumereplication-cr) replicationState
 from `primary` to `secondary` on the primary site.
* Scale down the applications on the secondary site.
* [Update VolumeReplication CR](rbd-mirroring.md#create-a-volumereplication-cr) replicationState state from `primary` to
 `secondary` in secondary site.
* On the primary site, [verify the VolumeReplication status](rbd-mirroring.md#checking-replication-status) is marked as
 volume ready to use.
* Once the volume is marked to ready to use, change the replicationState state
 from `secondary` to `primary` in primary site.
* Scale up the applications again on the primary site.
