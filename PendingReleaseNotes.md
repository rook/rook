# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- `CephObjectStoreUser` gained `spec.secretTemplate`, which adds labels and annotations to the Secret that holds the
  user's credentials, for example to opt the Secret in to a tool that copies Secrets into other namespaces. Keys that
  Rook sets or acts on are reserved.
- `CephBucketNotification` `spec.filter.metadataFilters` and `spec.filter.tagFilters` are now sent to RGW. They were previously accepted by the CRD and silently dropped, so notifications fired on objects the filters were meant to exclude.
