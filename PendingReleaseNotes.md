# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features
- `CephBucketNotification` `spec.filter.metadataFilters` and `spec.filter.tagFilters` are now sent to RGW. They were previously accepted by the CRD and silently dropped, so notifications fired on objects the filters were meant to exclude.
