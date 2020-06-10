---
title: Object Multisite
weight: 2250
indent: true
---
# NOTE

At the moment there is no realm pull feature implemented for the ceph-object-realm CR at the moment. Thus syncing objects between Ceph clusters is not working yet. Completing this feature is going to be completed by the Rook 1.4 release.

# Object Multisite

Multisite is a feature of Ceph that allows object stores to replicate its data over multiple Ceph clusters. 

Multisite also allows object stores to be independent and isolated from other object stores in a cluster.

When a ceph-object-store is created without the `zone` section; a realm, zone group, and zone is created with the same name as the ceph-object-store.

Since it is the only ceph-object-store in the realm, the data in the ceph-object-store remain independent and isolated from others on the same cluster.

When a ceph-object-store is created with the `zone` section, the ceph-object-store will join a custom created zone, zone group, and realm with a different than its own.

This allows the ceph-object-store to replicate its data over multiple Ceph clusters.

To review core multisite concepts please read the [ceph-multisite design overview](/design/ceph/object/ceph-multisite-overview.md).

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](ceph-quickstart.md).

# Creating Object Multisite

If an admin wants to set up multisite on a Rook Ceph cluster, the admin should create:

    - A [realm](/Documentation/ceph-object-multisite-crd.md)
    - A [zonegroup](/Documentation/ceph-object-multisite-crd.md)
    - A [zone](/Documentation/ceph-object-multisite-crd.md)
    - An [object-store](/Documentation/ceph-object-store.md) with the zone section

object-multisite.yaml in the [examples](/cluster/examples/kubernetes/ceph/) directory can be used to create the multisite CRDs.
```console
kubectl create -f object-multisite.yaml
```

The first zone group created in a realm is the master zone group. The first zone created in a zone group is the master zone.

When one of the multisite CRs (realm, zone group, zone) is deleted the underlying ceph realm/zone group/zone is not deleted. This must be done manually (see next section).

For more information on the multisite CRDs please read [ceph-object-multisite-crd](ceph-object-multisite-crd.md).

# Multisite Cleanup

## Realm Deletion

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

When the ceph-object-realm resource is deleted or modified, the realm is not deleted from the Ceph cluster. Realm deletion must be done via the toolbox.

### Deleting a Realm

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command. 

The following command, run via the toolbox, deletes the realm.

```console
radosgw-admin realm rm --rgw-realm=realm-a
```

## Zone Group Deletion

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

When the ceph-object-zone group resource is deleted or modified, the zone group is not deleted from the Ceph cluster. Zone Group deletion must be done through the toolbox.

### Deleting a Zone Group

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command. 

The following command, run via the toolbox, deletes the zone group.

```console
radosgw-admin zonegroup delete --rgw-realm=realm-a --rgw-zonegroup=zone-group-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zone-group=zone-group-a
```

## Deleting and Reconfiguring the Ceph Object Zone

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

When the ceph-object-zone resource is deleted or modified, the zone is not deleted from the Ceph cluster. Zone deletion must be done through the toolbox.

### Changing the Master Zone

The Rook toolbox can change the master zone in a zone group.

```console
radosgw-admin zone modify --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a --master
radosgw-admin zonegroup modify --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --master
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
```

### Deleting Zone

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command.

There are two scenarios possible when deleting a zone.
The following commands, run via the toolbox, deletes the zone if there is only one zone in the zone group.

```console
radosgw-admin zone rm --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
```

In the other scenario, there are more than one zones in a zone group.

Care must be taken when changing which zone is the master zone.

Please read the following [documentation](https://docs.ceph.com/docs/master/radosgw/multisite/#changing-the-metadata-master-zone) before running the below commands: 

The following commands, run via toolboxes, remove the zone from the zone group first, then delete the zone.

```console
radosgw-admin zonegroup rm --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
radosgw-admin zone rm --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zone-group=zone-group-a --rgw-zone=zone-a
```
