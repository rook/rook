# Rook Ceph Object Realm

## Prerequisites

A Rook Ceph cluster.

## Ceph Object Realm Walkthrough

The resource described in this design document represents the realm in the [Ceph Multisite data model](/design/ceph/object/ceph-multisite-overview.md).

### Creating an Object Realm

#### Config

When the storage admin is ready to create a multisite realm for object storage, the admin will name the realm in the metadata section on the configuration file.

This example `ceph-object-realm.yaml`, names a realm `my-realm`.
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectRealm
metadata:
  name: my-realm
  namespace: rook-ceph
```

Now create the object realm.
```bash
kubectl create -f ceph-object-realm.yaml
```

#### Steps

1. At this point the Rook operator recognizes that a new ceph-object-realm resource needs to be configured. The operator will start creating the ceph-object-realm resource.

2. After these steps the admin should create:
    - A [ceph-object-zone-group](/design/ceph/object/ceph-object-zone-group.md) referring to the ceph-object-realm resource.
    - A [ceph-object-zone](/design/ceph/object/ceph-object-zone.md) referring to the ceph-object-zone-group resource.
    - A [ceph-object-store](/design/ceph/object/ceph-object-store.md) referring to the ceph-object-zone resource.

The order in which these resources are created is not important.

3. Once all of the resources from step #2 are started up, the operator will create a realm on the Rook Ceph cluster and the ceph-realm resource will be running.

### Pulling the Realm from another Ceph Cluster

#### Config

When the storage admin is ready to sync data from a Ceph cluster with multisite set up (primary cluster) to another Rook Ceph cluster (pulling cluster), the pulling cluster needs to have information about the primary cluster's realm and zone group.

To do this, the storage admin needs to create a ceph-object-realm resource on the pulling cluster with the same name as the realm from the primary cluster.

The endpoint in the `pull` section is an endpoint of an object-store in the master zone of the realm.

This endpoint must be resolvable from the pulling cluster.

This example `ceph-object-realm-pulling.yaml`, pulls `my-realm` from a primary Ceph cluster with endpoint http://my-realm-zone-b-objectstore-1:80 in its master zone.
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectRealm
metadata:
  name: my-realm
  namespace: rook-ceph-2
spec:
  pull:
    endpoint: http://my-realm-zone-b-objectstore-1:80
```

Now create the ceph-object-realm
```console
kubectl create -f ceph-object-realm-pulling.yaml
```

Once the creation of the ceph-object-realm resource has been kicked off, the realm will be pulled on the pulling cluster.

If this realm pull succeeds, the ceph-object-realm resource is updated as Complete.

#### Steps
1. At this point the Rook operator recognizes that a new ceph-object-realm resource needs to be configured. Since the `pull` section has been specified, the operator will pull the realm.

3. After these steps the admin should create:
    1. A [ceph-object-zone](/design/ceph/ceph-object-zone.md) referring to the zone group the endpoint in the `pull` section is in.
    2. A [ceph-object-store](/design/ceph/ceph-object-store.md) referring to the newly created ceph-object-zone.

The order in which these resources are created is not important.

4. Once all of the resources steps #3 are started up and if the realm pull succeeds, the resource is updated as Complete and data will start syncing between the clusters.

#### Notes
1. The endpoint in `pull` must be resolvable from the pulling cluster.
2. The realm must have the same name in the `metadata` section, as the realm it is pulling from the multisite enabled Ceph cluster.

## Deleting and Reconfiguring the Object Realm

At the moment creating an ceph-object-realm resource only handles Day 1 initial configuration for the realm.

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

To be clear, when the ceph-object-realm resource is deleted or modified, the realm is not deleted from the Ceph cluster. Realm deletion must be done via the toolbox.

### Deleting a Realm

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command.

The following command, run via the toolbox, deletes the realm.

```console
radosgw-admin realm rm --rgw-realm=my-realm
```

## CephObjectRealm CRD

The ceph-object-realm settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources.

The name of the resource provided in the `metadata` section becomes the name of the realm.

If the `pull` section is included, the name in the `metadata` section must be the same as the realm it is trying to pull from.

The following variables can be configured in the ceph-object-realm resource:

- `pull`: This section is for any configuration for a realm being pulled from another cluster. The value of the `endpoint` in this section is of an endpoint for an ceph-object-store in the master zone of the realm being pulled. The endpoint must be resolvable from the cluster pulling the realm.

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectRealmPull
metadata:
  name: my-realm
  namespace: rook-ceph-2
spec:
  pull:
    endpoint: http://my-realm-zone-b-objectstore-1:80
```
