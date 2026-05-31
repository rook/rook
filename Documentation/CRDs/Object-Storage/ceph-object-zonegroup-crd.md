---
title: CephObjectZoneGroup CRD
---

Rook allows creation of zone groups in a [Ceph Object Multisite](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md)
configuration through a CRD. The following settings are available for Ceph object store zone groups.

## Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectZoneGroup
metadata:
  name: zonegroup-a
  namespace: rook-ceph
spec:
  realm: realm-a
```

## Settings

### Metadata

* `name`: The name of the object zone group to create
* `namespace`: The namespace of the Rook cluster where the object zone group is created.

### Spec

* `realm`: The object realm in which the zone group will be created. This matches the name of the object realm CRD.
