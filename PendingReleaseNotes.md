# v1.21 Pending Release Notes

## Breaking Changes

- Helm OCI chart tags no longer include the `v` prefix (e.g., `1.21.0` instead of `v1.21.0`). Update any scripts or tooling that reference the chart by tag.
- CephObjectStoreUsers created in a namespace other than their CephObjectStore (via the CephObjectStore `allowUsersInNamespaces` feature) can no longer be granted RGW admin capabilities (`spec.capabilities`) by default, because those capabilities are store-wide. Add the user's namespace to the CephObjectStore's new `allowAdminCapsInNamespaces` list to permit it. Users created in the object store's own namespace are unaffected.

## Features
