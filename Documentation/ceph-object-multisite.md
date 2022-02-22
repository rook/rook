---
title: Object Multisite
weight: 2250
indent: true
---
{% include_relative branch.liquid %}

# Object Multisite

Multisite is a feature of Ceph that allows object stores to replicate their data over multiple Ceph clusters.

Multisite also allows object stores to be independent and isolated from other object stores in a cluster.

When a ceph-object-store is created without the `zone` section; a realm, zone group, and zone is created with the same name as the ceph-object-store.

Since it is the only ceph-object-store in the realm, the data in the ceph-object-store remain independent and isolated from others on the same cluster.

When a ceph-object-store is created with the `zone` section, the ceph-object-store will join a custom created zone, zone group, and realm each with a different names than its own.

This allows the ceph-object-store to replicate its data over multiple Ceph clusters.

To review core multisite concepts please read the
[ceph-multisite design overview](https://github.com/rook/rook/blob/{{ branchName }}/design/ceph/object/ceph-multisite-overview.md).

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](quickstart.md).

# Creating Object Multisite

If an admin wants to set up multisite on a Rook Ceph cluster, the admin should create:

1. A [realm](ceph-object-multisite-crd.md#object-realm-settings)
1. A [zonegroup](ceph-object-multisite-crd.md#object-zone-group-settings)
1. A [zone](ceph-object-multisite-crd.md#object-zone-settings)
1. A ceph object store with the `zone` section

object-multisite.yaml in the [examples](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/) directory can be used to create the multisite CRDs.

```console
kubectl create -f object-multisite.yaml
```

The first zone group created in a realm is the master zone group. The first zone created in a zone group is the master zone.

When a non-master zone or non-master zone group is created, the zone group or zone is not in the Ceph Radosgw Multisite [Period](https://docs.ceph.com/docs/en/latest/radosgw/multisite/) until an object-store is created in that zone (and zone group).

The zone will create the pools for the object-store(s) that are in the zone to use.

When one of the multisite CRs (realm, zone group, zone) is deleted the underlying ceph realm/zone group/zone is not deleted, neither are the pools created by the zone. See the "Multisite Cleanup" section for more information.

For more information on the multisite CRDs please read [ceph-object-multisite-crd](ceph-object-multisite-crd.md).

# Pulling a Realm

If an admin wants to sync data from another cluster, the admin needs to pull a realm on a Rook Ceph cluster from another Rook Ceph (or Ceph) cluster.

To begin doing this, the admin needs 2 pieces of information:

1. An endpoint from the realm being pulled from
1. The access key and the system key of the system user from the realm being pulled from.

## Getting the Pull Endpoint

To pull a Ceph realm from a remote Ceph cluster, an `endpoint` must be added to the CephObjectRealm's `pull` section in the `spec`. This endpoint must be from the master zone in the master zone group of that realm.

If an admin does not know of an endpoint that fits this criteria, the admin can find such an endpoint on the remote Ceph cluster (via the tool box if it is a Rook Ceph Cluster) by running:

```
$ radosgw-admin zonegroup get --rgw-realm=$REALM_NAME --rgw-zonegroup=$MASTER_ZONEGROUP_NAME
```
>```
>{
>    ...
>    "endpoints": [http://10.17.159.77:80],
>    ...
>}
>```

A list of endpoints in the master zone group in the master zone is in the `endpoints` section of the JSON output of the `zonegoup get` command.

This endpoint must also be resolvable from the new Rook Ceph cluster. To test this run the `curl` command on the endpoint:

```
$ curl -L http://10.17.159.77:80
```
>```
><?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
>```

Finally add the endpoint to the `pull` section of the CephObjectRealm's spec. The CephObjectRealm should have the same name as the CephObjectRealm/Ceph realm it is pulling from.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectRealm
metadata:
  name: realm-a
  namespace: rook-ceph
spec:
  pull:
    endpoint: http://10.17.159.77:80
```

## Getting Realm Access Key and Secret Key

The access key and secret key of the system user are keys that allow other Ceph clusters to pull the realm of the system user.

### Getting the Realm Access Key and Secret Key from the Rook Ceph Cluster

When an admin creates a ceph-object-realm a system user automatically gets created for the realm with an access key and a secret key.

This system user has the name "$REALM_NAME-system-user". For the example realm, the uid for the system user is "realm-a-system-user".

These keys for the user are exported as a kubernetes [secret](https://kubernetes.io/docs/concepts/configuration/secret/) called "$REALM_NAME-keys" (ex: realm-a-keys).

To get these keys from the cluster the realm was originally created on, run:
```console
$ kubectl -n $ORIGINAL_CLUSTER_NAMESPACE get secrets realm-a-keys -o yaml > realm-a-keys.yaml
```
Edit the `realm-a-keys.yaml` file, and change the `namespace` with the namespace that the new Rook Ceph cluster exists in.

Then create a kubernetes secret on the pulling Rook Ceph cluster with the same secrets yaml file.
```console
kubectl create -f realm-a-keys.yaml
```

### Getting the Realm Access Key and Secret Key from a Non Rook Ceph Cluster

The access key and the secret key of the system user can be found in the output of running the following command on a non-rook ceph cluster:
```
radosgw-admin user info --uid="realm-a-system-user"
```
>```{
>    ...
>    "keys": [
>        {
>            "user": "realm-a-system-user"
>            "access_key": "aSw4blZIKV9nKEU5VC0="
>            "secret_key": "JSlDXFt5TlgjSV9QOE9XUndrLiI5JEo9YDBsJg==",
>        }
>    ],
>    ...
>}
>```

Then base64 encode the each of the keys and create a `.yaml` file for the Kubernetes secret from the following template.

Only the `access-key`, `secret-key`, and `namespace` sections need to be replaced.
```yaml
apiVersion: v1
data:
  access-key: YVN3NGJsWklLVjluS0VVNVZDMD0=
  secret-key: SlNsRFhGdDVUbGdqU1Y5UU9FOVhVbmRyTGlJNUpFbzlZREJzSmc9PQ==
kind: Secret
metadata:
  name: realm-a-keys
  namespace: $NEW_ROOK_CLUSTER_NAMESPACE
type: kubernetes.io/rook
```

Finally, create a kubernetes secret on the pulling Rook Ceph cluster with the new secrets yaml file.
```console
kubectl create -f realm-a-keys.yaml
```

### Pulling a Realm on a New Rook Ceph Cluster

Once the admin knows the endpoint and the secret for the keys has been created, the admin should create:

1. A [CephObjectRealm](ceph-object-multisite-crd.md#object-realm-settings) matching to the realm on the other Ceph cluster, with an endpoint as described above.
1. A [CephObjectZoneGroup](ceph-object-multisite-crd.md#object-zone-group-settings) matching the master zone group name or the master CephObjectZoneGroup from the cluster the the realm was pulled from.
1. A [CephObjectZone](ceph-object-multisite-crd.md#object-zone-settings) referring to the CephObjectZoneGroup created above.
1. A CephObjectStore referring to the new CephObjectZone resource.

object-multisite-pull-realm.yaml (with changes) in the [examples](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/) directory can be used to create the multisite CRDs.

```console
kubectl create -f object-multisite-pull-realm.yaml
```

# Multisite Cleanup

Multisite configuration must be cleaned up by hand. Deleting a realm/zone group/zone CR will not delete the underlying Ceph realm, zone group, zone, or the pools associated with a zone.

## Realm Deletion

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

When the ceph-object-realm resource is deleted or modified, the realm is not deleted from the Ceph cluster. Realm deletion must be done via the toolbox.

### Deleting a Realm

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command.

The following command, run via the toolbox, deletes the realm.

```console
radosgw-admin realm delete --rgw-realm=realm-a
```

## Zone Group Deletion

Changes made to the resource's configuration or deletion of the resource are not reflected on the Ceph cluster.

When the ceph-object-zone group resource is deleted or modified, the zone group is not deleted from the Ceph cluster. Zone Group deletion must be done through the toolbox.

### Deleting a Zone Group

The Rook toolbox can modify the Ceph Multisite state via the radosgw-admin command.

The following command, run via the toolbox, deletes the zone group.

```console
radosgw-admin zonegroup delete --rgw-realm=realm-a --rgw-zonegroup=zone-group-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zonegroup=zone-group-a
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
radosgw-admin zone delete --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
```

In the other scenario, there are more than one zones in a zone group.

Care must be taken when changing which zone is the master zone.

Please read the following [documentation](https://docs.ceph.com/docs/master/radosgw/multisite/#changing-the-metadata-master-zone) before running the below commands:

The following commands, run via toolboxes, remove the zone from the zone group first, then delete the zone.

```console
radosgw-admin zonegroup rm --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
radosgw-admin zone delete --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
radosgw-admin period update --commit --rgw-realm=realm-a --rgw-zonegroup=zone-group-a --rgw-zone=zone-a
```

When a zone is deleted, the pools for that zone are not deleted.

### Deleting Pools for a Zone

The Rook toolbox can delete pools. Deleting pools should be done with caution.

The following [documentation](https://docs.ceph.com/docs/master/rados/operations/pools/) on pools should be read before deleting any pools.

When a zone is created the following pools are created for each zone:
```
$ZONE_NAME.rgw.control
$ZONE_NAME.rgw.meta
$ZONE_NAME.rgw.log
$ZONE_NAME.rgw.buckets.index
$ZONE_NAME.rgw.buckets.non-ec
$ZONE_NAME.rgw.buckets.data
```
Here is an example command to delete the .rgw.buckets.data pool for zone-a.

```console
ceph osd pool rm zone-a.rgw.buckets.data zone-a.rgw.buckets.data --yes-i-really-really-mean-it
```

In this command the pool name **must** be mentioned twice for the pool to be removed.

### Removing an Object Store from a Zone

When an object-store (created in a zone) is deleted, the endpoint for that object store is removed from that zone, via
```console
kubectl delete -f object-store.yaml
```

Removing object store(s) from the master zone of the master zone group should be done with caution. When all of these object-stores are deleted the period cannot be updated and that realm cannot be pulled.
