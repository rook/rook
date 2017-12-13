# Major Themes

## Action Required

## Notable Features

## Breaking Changes

### Cluster CRD
- Removed the `versionTag` property. The container version to launch in all pods will be the same as the version of the operator container. 

### Operator
- Removed the `ROOK_REPO_PREFIX` env var. All containers will be launched with the same image as the operator

## Known Issues

## Deprecations
