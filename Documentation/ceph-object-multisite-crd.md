---
title: Object Multisite CRDs
weight: 2825
indent: true
---

# Ceph Object Multisite CRDs

The following CRDs enable Ceph object stores to isolate or replicate data via multisite. For more information on multisite, visit the [ceph-object-multisite](/Documentation/ceph-object-multisite.md) documentation.

## Ceph Object Realm CRD

Rook allows creation of a realm in a ceph cluster for object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object store realms.

### Sample

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectRealm
metadata:
  name: realm-a
  namespace: rook-ceph
```

### Object Realm Settings

#### Metadata

* `name`: The name of the object realm to create
* `namespace`: The namespace of the Rook cluster where the object realm is created.

## Ceph Object Zone Group CRD

Rook allows creation of zone groups in a ceph cluster for object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object store zone groups.

### Sample

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

### Sample

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectZone
metadata:
  name: zone-a
  namespace: rook-ceph
spec:
  zonegroup: zonegroup-a
```

### Object Zone Settings

#### Metadata

* `name`: The name of the object zone to create
* `namespace`: The namespace of the Rook cluster where the object zone is created.

#### Spec

* `zonegroup`: The object zonegroup in which the zone will be created. This matches the name of the object zone group CRD.
