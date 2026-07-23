# RGW User Multitenancy and Default Placement Targeting in CephObjectStoreUser

- **Issue**: https://github.com/rook/rook/issues/17274

## Summary

This document proposes extending the `CephObjectStoreUser` CRD with four new spec fields:

- `tenant` — assigns the RGW user to a named RGW tenant, enabling bucket name isolation across tenants.
- `defaultPlacement` — sets the user's default bucket placement target, controlling which data/metadata pools newly created buckets land in.
- `defaultStorageClass` — sets the user's default storage class for objects, applied on top of `defaultPlacement`.
- `defaultPlacementTags` — sets storage class placement tags associated with the user's default placement.

All four fields already exist in the underlying `admin.User` struct in `go-ceph`; this work wires them into the Rook controller and API.

### Why tenant and placement are covered in the same document

`tenant`, `defaultPlacement`, `defaultStorageClass`, and `defaultPlacementTags` are unrelated in what they do in RGW, but they're proposed together here because they:

- are all optional additions to the exact same CRD field (`ObjectStoreUserSpec`), reviewed against the same schema and the same immutability/mutability rules,
- share the same controller entry points (`generateUserConfig`, `isUserSync`, `createOrUpdateCephUser`), so a reviewer needs to see how they compose in that code regardless of which section of the doc they came from,

Splitting placement into its own document would not change the schema or the controller logic — it would only move prose. We've kept them together so the field interactions (e.g. `defaultStorageClass` requiring `defaultPlacement`, all three being independent of `tenant`) are visible in one place instead of cross-referenced across two documents.

## Motivation

### Tenant Isolation

Ceph RGW supports a multitenancy model where users live in named tenants. Users in different tenants can own buckets with the same name without collision:

```
# Two separate objects, no conflict
tenantA$user1 → s3://photos
tenantB$user1 → s3://photos
```

Rook currently has no mechanism to place a `CephObjectStoreUser` in an RGW
tenant. Operators who need per-tenant user isolation must manage RGW users
manually outside of Rook, forgoing the benefits of the operator (secret
rotation, lifecycle management, status conditions).

### Placement Targeting

`CephObjectStore.spec.sharedPools.poolPlacements` already allows defining named
placement targets (each backed by distinct metadata/data pools). However, the
`CephObjectStoreUser` controller has no way to assign a user's
`default-placement`, meaning all users default to the store-wide default
placement.

## Goals

- Add `spec.tenant` to `CephObjectStoreUser` to set the `Tenant` field on the RGW Admin Ops API `User` struct at user creation.
- Add `spec.defaultPlacement` to `CephObjectStoreUser` to set `DefaultPlacement` on the user via `CreateUser`/`ModifyUser` in the RGW Admin Ops API.
- Add `spec.defaultStorageClass` to `CephObjectStoreUser` to set `DefaultStorageClass` on the user, embedded into the placement rule sent to RGW
- Add `spec.defaultPlacementTags` to `CephObjectStoreUser` to set `PlacementTags` on the user via `CreateUser`/`ModifyUser`, now that go-ceph supports it upstream (merged in [ceph/go-ceph#1290](https://github.com/ceph/go-ceph/pull/1290)).
- Validate `defaultPlacement` against the named placements defined in the referenced `CephObjectStore`.
- Require `defaultPlacement` to be set whenever `defaultStorageClass` is set, since RGW cannot apply a storage class without a placement target.
- Treat `tenant` as immutable (changing tenant requires user deletion and recreation in RGW).
- Treat `defaultPlacement`, `defaultStorageClass`, and `defaultPlacementTags` as mutable (can be updated via `ModifyUser`).
- Explicitly reconcile removal of `defaultPlacement`/`defaultStorageClass`/`defaultPlacementTags` back to Ceph's defaults rather than leaving the RGW-side value stale (see [Unset/Removal Behavior](#unsetremoval-behavior)).
- All four fields are independent of `tenant`: `defaultPlacement`/`defaultStorageClass`/`defaultPlacementTags` may be set without `tenant`, and vice versa.
- Preserve backward compatibility: all new fields are optional, existing resources are unaffected.


## Background

### RGW Tenant User ID Format

When a user is created in a tenant, RGW stores and returns the user as `$tenant$uid` internally, but the external UID (passed to the API) is just `uid`. When querying or deleting a tenanted user, the RGW Admin Ops API accepts a `Tenant` field alongside `ID` rather than a combined string.

Equivalently via CLI:
```bash
radosgw-admin user create --uid="user1" --tenant="tenantA" --display-name="User 1"
# effective UID internally: tenantA$user1

radosgw-admin user info --uid="user1" --tenant="tenantA"
```

The `go-ceph` `admin.User` struct already models this:
```go
type User struct {
    ID                  string   `json:"user_id" url:"uid"`
    Tenant              string   `url:"tenant"`                                          // ← passed as URL param only, not in JSON response
    DefaultPlacement    string   `json:"default_placement" url:"default-placement"`
    DefaultStorageClass string   `json:"default_storage_class" url:"default-storage-class"`
    PlacementTags       []string `json:"placement_tags" url:"placement-tags"`             // ← support merged upstream in ceph/go-ceph#1290
    // ...
}
```


### Interaction with `AccountRef`

`AccountRef` (added in a recent release) also links users to an RGW account and is already marked immutable. Users with `accountRef` set are account-member users. Tenant assignment and account membership are orthogonal in RGW—a user can belong to both a tenant and an account.

## Proposed API Changes

### `ObjectStoreUserSpec` (`pkg/apis/ceph.rook.io/v1/types.go`)

`defaultPlacement` and `defaultStorageClass` are added as flat, top-level `*string` fields directly on `ObjectStoreUserSpec`, named to match the `go-ceph` `admin.User` fields exactly (`DefaultPlacement`, `DefaultStorageClass`), rather than being wrapped in a nested `ObjectStoreUserPlacementSpec` struct. This mirrors the pattern already established by PR [#17260](https://github.com/rook/rook/pull/17260), which added `DefaultStorageClass` as a flat field on the same spec — introducing a nested struct here would create two incompatible shapes for closely related fields on the same object.

```go
// ObjectStoreUserSpec represent the spec of an Objectstoreuser
// +kubebuilder:validation:XValidation:message="defaultStorageClass requires defaultPlacement",rule="!has(self.defaultStorageClass) || has(self.defaultPlacement)"
type ObjectStoreUserSpec struct {
    Store        string `json:"store,omitempty"`
    DisplayName  string `json:"displayName,omitempty"`
    // ... existing fields ...

    // Tenant is the RGW tenant this user belongs to.
    // Users in different tenants can have buckets with the same name without conflict.
    // When set, the effective user ID in RGW becomes "<tenant>$<name>".
    // This field is immutable after creation.
    // +optional
    // +kubebuilder:validation:XValidation:message="tenant is immutable",rule="self == oldSelf"
    // +kubebuilder:validation:Pattern=`^[a-zA-Z0-9._-]+$`
    // +kubebuilder:validation:MaxLength=255
    Tenant string `json:"tenant,omitempty"`

    // DefaultPlacement overrides the default pool placement for buckets created by
    // this user. Must match one of the entries in the referenced CephObjectStore's
    // spec.sharedPools.poolPlacements[].name. If not provided, the zone group's
    // default placement target is used.
    // +optional
    // +kubebuilder:validation:MinLength=0
    // +kubebuilder:validation:MaxLength=2048
    DefaultPlacement *string `json:"defaultPlacement,omitempty"`

    // DefaultStorageClass overrides the default storage class for objects created by
    // this user. Requires DefaultPlacement to be set. If not provided, the default
    // `STANDARD` storage class is used.
    // +optional
    // +kubebuilder:validation:MinLength=0
    // +kubebuilder:validation:MaxLength=2048
    DefaultStorageClass *string `json:"defaultStorageClass,omitempty"`

    // DefaultPlacementTags is a list of storage class tags to associate with this
    // user's default placement.
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=64
    DefaultPlacementTags []string `json:"defaultPlacementTags,omitempty"`
}
```

Maps to `go-ceph` `admin.User` fields:
- `DefaultPlacement` → `DefaultPlacement` (URL param `default-placement`, embeds storage class — see below)
- `DefaultStorageClass` → `DefaultStorageClass` (URL param `default-storage-class`, informational only — see below)
- `DefaultPlacementTags` → `PlacementTags` (URL param `placement-tags`, JSON `placement_tags`)

**`PlacementTags` was previously out of scope** because go-ceph did not yet support it and PR #17260 does not implement it. That gap has since closed: [ceph/go-ceph#1290](https://github.com/ceph/go-ceph/pull/1290) added `PlacementTags` support to the admin ops client and merged upstream on 2026-07-09. This design now includes it as a flat `DefaultPlacementTags []string` field, following the same flat, go-ceph-aligned naming as the other fields. Rook's `go.mod` currently points at a fork (`github.com/ideepika/go-ceph`) pending an official tagged go-ceph release that includes this change; that pin must be replaced with a released version before this can merge.

### Unset/Removal Behavior

Ceph RGW's `ModifyUser` semantics do not treat an absent field as "no change" —
the field's zero value is sent, and RGW acts on it:

- If `defaultPlacement` is set and later removed from the spec, the controller
  must issue a `ModifyUser` call with an explicit empty `default-placement`,
  which reverts the user to the zone group's default placement target. Simply
  omitting the field from a subsequent reconcile is not sufficient — go-ceph's
  `ModifyUser` sends whatever value is present on the `admin.User` struct
  passed to it, so a removed spec field must be explicitly reconciled by the
  controller (e.g. by constructing the request with `DefaultPlacement: ""`),
  not just left unset in Go.

- If both are removed together, the controller reverts to sending an empty
  `default-placement`, which implicitly also clears any storage class override.
- If `defaultPlacementTags` is set and later removed, the controller must send
  an empty `placement-tags` list, clearing the tags on the live RGW user rather
  than leaving the last-applied tags in place.

This mirrors the enforcement mechanism already used for `tenant`'s
immutability: the desired end-state is expressed declaratively in the spec, and
the controller is responsible for issuing whatever explicit RGW admin API call
is needed to converge live state to it, including reverting fields to their
Ceph-side defaults on removal — not merely for what to send when a field is
populated.

### Example CR

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: user1
  namespace: rook-ceph
spec:
  store: my-store
  displayName: "Tenant A User 1"
  tenant: tenantA
  defaultPlacement: hot-tier
  defaultStorageClass: STANDARD_IA
  defaultPlacementTags:
    - tenant-a
```

## S3 Client Configuration for Tenanted Users

RGW exposes tenanted users to S3 clients through their access key / secret key pair — the S3 client itself requires no special modification. Credentials stored in the Rook-managed Kubernetes Secret are functionally identical regardless of whether the user belongs to a tenant.

```yaml
# AWS CLI profile for a tenanted user — identical to a non-tenanted user
[profile tenantA-user1]
aws_access_key_id     = <AccessKey from rook-ceph-object-user-my-store-user1>
aws_secret_access_key = <SecretKey from rook-ceph-object-user-my-store-user1>
```

### Intra-tenant access (primary use case)

Users within the same tenant access their buckets using standard S3 virtual-host-style URLs with no changes:

```
my-bucket.s3.ceph.io   ← works normally for same-tenant users
```

RGW resolves the bucket to the correct tenant namespace based on the credentials used. No DNS changes or special endpoint configuration are required for this feature's primary use case.

### Cross-tenant access (out of scope, deprecated upstream)

Cross-tenant bucket access via path-style requests using the `tenant:bucket`
notation (e.g. `s3.ceph.io/tenantA:my-bucket/`) is a Ceph extension to the S3
protocol. As noted in the Ceph Tentacle release notes, this feature is
deprecated and scheduled for removal.

> S3 API support for cross-tenant names such as `Bucket='tenant:bucketname'`

Virtual-host-style cross-tenant access (`tenantA:my-bucket.s3.ceph.io`) is not
possible because `:` is not valid in DNS names.

**Cross-tenant bucket sharing is explicitly out of scope for this feature.**
Users who need to share buckets across tenant boundaries should be placed in
the same tenant namespace. This aligns with Ceph's upstream direction of
removing cross-tenant path-style access.

## Immutability

`tenant` is immutable because RGW does not support moving a user between
tenants; the only path is deletion and recreation. Attempting to change
`tenant` on an existing `CephObjectStoreUser` would silently create a second
user in the new tenant while leaving the original orphaned.


`defaultPlacement` and `defaultStorageClass` are mutable — RGW supports
changing a user's default placement and storage class at any time; changes only
affect future bucket/object creation, not existing buckets/objects. As
described in [Unset/Removal Behavior](#unsetremoval-behavior), mutability also
covers the removal case: the controller must actively drive RGW back to its own
defaults rather than treating an unset field as a no-op.

## Interaction with `AccountRef`

`tenant` and `accountRef` are orthogonal. Both can be set simultaneously. When both are set:
- `userConfig.Tenant` is set from `spec.tenant`
- `userConfig.AccountID` is set from `spec.accountRef`

No conflict; `admin.User` has both fields.
