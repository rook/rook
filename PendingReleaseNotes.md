# Major Themes

## Action Required

## Notable Features
- Creation of storage pools through the custom resource definitions (CRDs) now allows users to optionally specify `deviceClass` property to enable
distribution of the data only across the specified device class. See [Ceph Block Pool CRD](Documentation/ceph-pool-crd.md#ceph-block-pool-crd) for
an example usage

### Ceph

- Rook can now be configured to read "region" and "zone" labels on Kubernetes nodes and use that information as part of the CRUSH location for the OSDs.
- Rgw pods have liveness probe enabled

## Breaking Changes

### <Storage Provider>

## Known Issues

### <Storage Provider>

## Deprecations

### <Storage Provider>
