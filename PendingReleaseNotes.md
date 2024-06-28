# v1.15 Pending Release Notes

## Breaking Changes
- During CephBlockPool updates, return an error if an invalid device class is specified. Pools with invalid device classes may start failing reconcile until the correct device class is specified. See #14057.

## Features
