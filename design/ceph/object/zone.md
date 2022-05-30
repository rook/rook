# Rook Ceph Object Zone

## Prerequisites

A Rook Ceph cluster. Ideally a ceph-object-realm and a ceph-object-zone-group resource would have been started up already.

## Ceph Object Zone Walkthrough

The resource described in this design document represents the zone in the [Ceph Multisite data model](/design/ceph/object/ceph-multisite-overview.md).

### Creating an Ceph Object Zone

#### Config

When the storage admin is ready to create a multisite zone for object storage, the admin will name the zone in the metadata section on the configuration file.

In the config, the admin must configure the zone group the zone is in, and pools for the zone.

The first zone created in a zone group is designated as the master zone in the Ceph cluster.

This example `ceph-object-zone.yaml`, names a zone `my-zone`.
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectZone
metadata:
  name: zone-a
  namespace: rook-ceph
spec:
  zoneGroup: zone-group-b
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
```

Now create the ceph-object-zone.
```bash
kubectl create -f ceph-object-zone.yaml
```

#### Steps

1. At this point the Rook operator recognizes that a new ceph-object-zone resource needs to be configured. The operator will start creating the resource to start the ceph-object-zone.

2. After these steps the admin should start up:
    - A [ceph-object-store](/design/ceph/object/ceph-object-store.md) referring to the newly started up ceph-object-zone resource.
    - A [ceph-object-zone-group](/design/ceph/object/ceph-object-zone-group.md), with the same name as the `zoneGroup` field, if it has not already been started up already.
    - A [ceph-object-realm](/design/ceph/object/ceph-object-realm.md), with the same name as the `realm` field in the ceph-object-zone-group config, if it has not already been started up already.

The order in which these resources are created is not important.

3. Once the all of the resources in #2 are started up, the operator will create a zone on the Rook Ceph cluster and the ceph-object-zone resource will be running.

#### Notes

1. The zone group named in the `zoneGroup` section must be the same as the ceph-object-zone-group resource the zone is a part of.
2. When resource is deleted, zone are not deleted from the cluster. zone deletion must be done through toolboxes.
3. Any number of ceph-object-stores can be part of a ceph-object-zone.

### Creating an ceph-object-zone when syncing from another Ceph Cluster

When the storage admin is ready to sync data from another Ceph cluster with multisite set up (primary cluster) to a Rook Ceph cluster (pulling cluster), the pulling cluster will have a newly created in the zone group from the primary cluster.

A [ceph-object-pull-realm](/design/object/ceph-object-pull-realm.md) resource must be created to pull the realm information from the primary cluster to the pulling cluster.

Once the ceph-object-pull-realm is configured a ceph-object-zone must be created.

After an ceph-object-store is configured to be in this ceph-object-zone, the all Cehph multisite resources will be running and data between the two clusters will start syncing.

## Deleting and Reconfiguring the Ceph Object Zone

At the moment creating an ceph-object-zone resource only handles Day 1 initial configuration for the zone.

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

To be clear, when the ceph-object-zone resource is deleted or modified, the zone is not deleted from the Ceph cluster. Zone deletion must be done through the toolbox.

### Deleting Zone through Toolboxes

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command.

There are two scenarios possible when deleting a zone.
The following commands, run via the toolbox, deletes the zone if there is only one zone in the zone group.

```bash
# radosgw-admin zone rm --rgw-zone=zone-z
# radosgw-admin period update --commit
```

In the other scenario, there are more than one zones in a zone group.

Care must be taken when changing which zone is the master zone.

Please read the following documentation before running the below commands:

https://docs.ceph.com/docs/master/radosgw/multisite/#changing-the-metadata-master-zone

The following commands, run via toolboxes, remove the zone from the zone group first, then delete the zone.

```bash
# radosgw-admin zonegroup rm --rgw-zone=zone-z
# radosgw-admin period update --commit
# radosgw-admin zone rm --rgw-zone=zone-z
# radosgw-admin period update --commit
```
### Changing the Master Zone through Toolboxes

Similar to deleting zones, the Rook toolbox can also change the master zone in a zone group.

```bash
# radosgw-admin zone modify --rgw-zone=zone-z --master
# radosgw-admin zonegroup modify --rgw-zonegroup=zone-group-b --master
# radosgw-admin period update --commit
```

## CephObjectZone CRD

The ceph-object-zone settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources.

The name of the resource provided in the `metadata` section becomes the name of the zone.

The following variables can be configured in the ceph-object-zone resource.

- `zoneGroup`: The zone group named in the `zoneGroup` section of the ceph-realm resource the zone is a part of.

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectZone
metadata:
  name: zone-b
  namespace: rook-ceph
spec:
  zoneGroup: zone-group-b
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
```

### Pools

The pools are the backing data store for the object stores in the zone and are created with specific names to be private to a zone.
As long as the `zone` config option is specified in the object-store's config, the object-store will use pools defined in the ceph-zone's configuration.
Pools can be configured with all of the settings that can be specified in the [Pool CRD](/Documentation/CRDs/Block-Storage/ceph-block-pool-crd.md).
The underlying schema for pools defined by a pool CRD is the same as the schema under the `metadataPool` and `dataPool` elements of the object store CRD.
All metadata pools are created with the same settings, while the data pool can be created with independent settings.
The metadata pools must use replication, while the data pool can use replication or erasure coding.

When the ceph-object-zone is deleted the pools used to support the zone will remain just like the zone. This is a security measure to avoid accidental loss of data.

Just like deleting the zone itself, removing the pools must be done by hand through the toolbox.

```yaml
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
```
