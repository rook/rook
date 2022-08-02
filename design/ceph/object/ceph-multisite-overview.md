## Ceph Multisite Overview

Multisite is a feature of Ceph that allows object stores to replicate its data over multiple Ceph clusters.

Multisite also allows object stores to be independent and isolated from other object stores in a cluster.

### Ceph Multisite data model

For reference, here is a description of the underlying Ceph Multisite data model.

```
A cluster has one or more realms

A realm spans one or more clusters
A realm has one or more zone groups
A realm has one master zone group
A realm defined in another cluster is replicated with the pull command
The objects in a realm are independent and isolated from objects in other realms

A zone group has one or more zones
A zone group has one master zone
A zone group spans one or more clusters
A zone group defined in another cluster is replicated with the pull command
A zone group defines a namespace for object IDs unique across its zones
Zone group metadata is replicated to other zone groups in the realm

A zone belongs to one cluster
A zone has a set of pools that store the user and object metadata and object data
Zone data and metadata is replicated to other zones in the zone group
A master zone needs to be created for secondary zones to pull from to replicate across zones
```

When a ceph-object-store is created without the `zone` section; a realm, zone group, and zone is created with the same name as the ceph-object-store.

Since it is the only ceph-object-store in the realm, the data in the ceph-object-store remain independent and isolated from others on the same cluster.

When a ceph-object-store is created with the `zone` section, the Ceph Multisite will be configured.

The ceph-object-store will join a zone, zone group, and realm with a different than it's own.

This allows the ceph-object-store to replace it's data over multiple Ceph clusters.

### Overview Ceph Multisite Steps
To enable Ceph's multisite, the following steps need to happen.

1. A realm needs to be created
2. A master zone group in the realm needs to be created
3. A master zone in the master zone group needs to be created
4. An object store needs to be added to the master zone

### Master Zone/Zonegroup
The master zone of the master zonegroup is designated as the 'metadata master zone', and all changes to user and bucket metadata are written through that zone first before replicating to other zones via metadata sync. This is different from data sync, where objects can be written to any zone and replicated to its peers in the zonegroup.

## Rook Ceph Multisite Steps

1. If an admin is creating a new realm on a Rook Ceph cluster, the admin should create:

    - A [ceph-object-realm](/design/ceph/object/realm.md) with the name of the realm the admin wishes to create.
    - A [ceph-object-zone-group](/design/ceph/object/zone-group.md) referring to the ceph-object-realm resource.
    - A [ceph-object-zone](/design/ceph/object/zone.md) referring to the ceph-object-zone-group resource.
    - A [ceph-object-store](/design/ceph/object/store.md) referring to the ceph-object-zone resource.

2. If an admins pulls a realm on a Rook Ceph cluster from another Ceph cluster, the admin should create:

    - A [ceph-object-realm](/design/ceph/object/realm.md) referring to the realm on the other Ceph cluster, and an endpoint in a master zone in that realm.
    - A [ceph-object-zone-group](/design/ceph/object/zone-group.md) referring to the realm that was pulled or matching the ceph-object-zone-group resource from the cluster the realm was pulled from.
    - A [ceph-object-zone](/design/ceph/object/zone.md) referring to the zone group that the new zone will be in.
    - A [ceph-object-store](/design/ceph/object/store.md) referring to the ceph-object-zone resource.

### Future Design Roadmap

At the moment the multisite resources only handles Day 1 initial configuration.

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

To be clear, when the ceph-object-{realm, zone group, zone} resource is deleted or modified, the realm/zone group/zone is not deleted or modified in the Ceph cluster. Deletion or modification must be done the toolbox.

Future iterations of this design will address these Day 2 operations and other such as:

- Initializing and modifying Storage Classes
- Deletion of the CR reflecting deletion of the realm, zone group, & zone
- The status of the ceph-object-{realm, zone group, zone} reflecting the status of the realm, zone group, and zone.
- Changing the master zone group in a realm
- Changing the master zone in a zone group
