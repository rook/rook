# Major Themes

## Action Required

## Known Issues

## Deprecations

## Notable Features

- Object Store
  - Object Stores are defined by a CRD and handled by the Operator
  - Multiple object stores supported through Ceph realms
  - Pools created by object stores are configurable with all options defined in the pool CRD

## Breaking Changes

- Rook Standalone
  - Standalone mode has been disabled and is no longer supported. A Kubernetes environment is required to run Rook.
- Pool CRD
  - `replication` renamed to `replicated`
  - `erasureCode` renamed to `erasureCoded`