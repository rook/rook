---
title: Object Multisite CRDs
---

The following CRDs enable Ceph object stores to isolate or replicate data via multisite. For more information on multisite, visit the [Ceph Object Multisite CRDs documentation](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md).

## Ceph Object Realm CRD

Rook allows creation of a realm in a ceph cluster for object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object store realms.

### Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectRealm
metadata:
  name: realm-a
  namespace: rook-ceph
# This endpoint in this section needs is an endpoint from the master zone  in the master zone group of realm-a. See object-multisite.md for more details.
spec:
  pull:
    endpoint: http://10.2.105.133:80
```

### Object Realm Settings

#### Metadata

* `name`: The name of the object realm to create
* `namespace`: The namespace of the Rook cluster where the object realm is created.

#### Spec

* `pull`: This optional section is for the pulling the realm for another ceph cluster.
  * `endpoint`: The endpoint in the realm from another ceph cluster you want to pull from. This endpoint must be in the master zone of the master zone group of the realm.

## Ceph Object Zone Group CRD

Rook allows creation of zone groups in a ceph cluster for object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object store zone groups.

### Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectZoneGroup
metadata:
  name: zonegroup-a
  namespace: rook-ceph
spec:
  realm: realm-a
```

### Object Zone Group Settings

#### Metadata

* `name`: The name of the object zone group to create
* `namespace`: The namespace of the Rook cluster where the object zone group is created.

#### Spec

* `realm`: The object realm in which the zone group will be created. This matches the name of the object realm CRD.

## Ceph Object Zone CRD

Rook allows creation of zones in a ceph cluster for object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object store zone.

### Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectZone
metadata:
  name: zone-a
  namespace: rook-ceph
spec:
  zoneGroup: zonegroup-a
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: osd
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
```

### Object Zone Settings

#### Metadata

* `name`: The name of the object zone to create
* `namespace`: The namespace of the Rook cluster where the object zone is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](../Block-Storage/ceph-block-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

#### Spec

* `zonegroup`: The object zonegroup in which the zone will be created. This matches the name of the object zone group CRD.
* `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
* `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.
