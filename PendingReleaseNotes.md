# Major Themes

## Action Required

## Known Issues

## Deprecations

## Notable Features

- Object Store
  - Object Stores are defined by a CRD and handled by the Operator
  - Multiple object stores supported through Ceph realms
  - Pools created by object stores are configurable with all options defined in the pool CRD
- OSDs
  - If an OSD loses its metadata and config but still has its data devices, the OSD will automatically regenerate the lost metadata to make the data available again.

## Breaking Changes

- Rook Standalone
  - Standalone mode has been disabled and is no longer supported. A Kubernetes environment is required to run Rook.
- Pool CRD
  - `replication` renamed to `replicated`
  - `erasureCode` renamed to `erasureCoded`
- OSDs
  - OSD pods now require RBAC permissions to create/get/update/delete/list config maps.
  An upgraded operator will create the necessary service account, cluster role, and cluster role bindings to enable this.
