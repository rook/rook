# v1.19 Pending Release Notes

## Breaking Changes

- The behavior of the `activeStandby` property in the `CephFilesystem` CRD has changed.
    When set to `false`, the standby MDS daemon deployment will be scaled down and removed,
    rather than only disabling the standby cache while the daemon remains running.

## Features
