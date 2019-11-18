---
title: Object Store User CRD
weight: 2900
indent: true
---

# Ceph Object Store User CRD

Rook allows creation and customization of object store users through the custom resource definitions (CRDs). The following settings are available
for Ceph object store users.

## Sample

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: my-display-name
```

## Object Store User Settings

### Metadata

* `name`: The name of the object store user to create, which will be reflected in the secret and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store user is created.

### Spec

* `store`: The object store in which the user will be created. This matches the name of the objectstore CRD.
* `displayName`: The display name which will be passed to the `radosgw-admin user create` command.
