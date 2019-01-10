# Major Themes


## Action Required


## Notable Features

- Minio Object Stores:
    - Now have an additional label named `objectstore` with the name of the Object Store, to allow better selection for Services.
    - Use `Readiness` and `Liveness` probes.
    - Updated automatically on Object Store CRD changes.
- Ceph:
    - `mon_max_pgs_per_osd` is risky for Rook to set by default and has been removed from the Ceph config

## Breaking Changes

- Rook no longer supports Kubernetes `1.8` and `1.9`.

## Known Issues


## Deprecations
