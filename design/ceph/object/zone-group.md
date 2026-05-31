# Rook Ceph Object Zone Group

## Prerequisites

A Rook Ceph cluster. Ideally a ceph-object-realm resource would have been started up already.

## Ceph Object Zone Group Walkthrough

The resource described in this design document represents the zone group in the [Ceph Multisite data model](/design/ceph/object/ceph-multisite-overview.md).


### Creating an Ceph Object Zone Group

#### Config

When the storage admin is ready to create a multisite zone group for object storage, the admin will name the zone group in the metadata section on the configuration file.

In the config, the admin must configure the realm the zone group is in.

The first ceph-object-zone-group resource created in a realm is designated as the master zone group in the Ceph cluster.

This example `ceph-object-zone-group.yaml`, names a zone group `my-zonegroup`.
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectZoneGroup
metadata:
  name: zone-group-a
  namespace: rook-ceph
spec:
  realm: my-realm
```

Now create the ceph-object-zone-group.
```bash
kubectl create -f ceph-object-zone-group.yaml
```

#### Steps

1. At this point the Rook operator recognizes that a new ceph-object-zone-group resource needs to be configured. The operator will start creating the resource to start the ceph-object-zone-group.

2. After these steps the admin should start up:
    - A [ceph-object-zone](/design/ceph/object/ceph-object-zone.md) with the name of the zone group in the `zoneGroup` section.
    - A [ceph-object-store](/design/ceph/object/ceph-object-store.md) referring to the newly started up ceph-object-zone resource.
    - A [ceph-object-realm](/design/ceph/object/ceph-object-realm.md), with the same name as the `realm` field, if it has not already been started up already.

The order in which these resources are created is not important. 

3. Once all of the resources in #2 are started up, the operator will create a zone group on the Rook Ceph cluster and the ceph-object-zone-group resource will be running.

#### Notes

1. The realm named in the `realm` section must be the same as the ceph-object-realm resource the zone group is a part of.
3. When resource is deleted, zone group are not deleted from the cluster. Zone group deletion must be done through toolboxes.

## Deleting and Reconfiguring the Ceph Object Zone Group

At the moment creating an ceph-object-zone-group realm resource only handles Day 1 initial configuration for the realm. 

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

To be clear, when the ceph-object-zone group resource is deleted or modified, the zone group is not deleted from the Ceph cluster. Zone Group deletion must be done through the toolbox.

### Deleting a Zone Group through Toolboxes

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command. 

The following command, run via the toolbox, deletes the zone group.

```bash
# radosgw-admin zonegroup delete --rgw-zonegroup=zone-group-b
# radosgw-admin period update --commit
```

## CephObjectZoneGroup CRD

The ceph-object-zone-group settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources.

The name of the resource provided in the `metadata` section becomes the name of the zone group.

The following variables can be configured in the ceph-zone-group resource.

- `realm`: The realm named in the `realm` section of the ceph-realm resource the zone group is a part of.

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectZoneGroup
metadata:
  name: zone-group-b
  namespace: rook-ceph
spec:
  realm: my-realm
```
