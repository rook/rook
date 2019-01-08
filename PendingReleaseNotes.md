# Major Themes


## Action Required


## Notable Features

- Minio Object Stores:
    - Now have an additional label named `objectstore` with the name of the Object Store, to allow better selection for Services.
    - Use `Readiness` and `Liveness` probes.
    - Updated automatically on Object Store CRD changes.
    
- Selinux labeling for mounts can now be toggled with the ROOK_ENABLE_SELINUX_RELABELING environment variable. This addresses https://github.com/rook/rook/issues/2417

## Breaking Changes

- Rook no longer supports Kubernetes `1.8` and `1.9`.

## Known Issues


## Deprecations
