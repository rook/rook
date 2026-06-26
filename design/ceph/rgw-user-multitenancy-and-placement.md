# RGW User Multitenancy and Default Placement Targeting in CephObjectStoreUser

- **Issue**: https://github.com/rook/rook/issues/17274

## Summary

This document proposes extending the `CephObjectStoreUser` CRD with two new spec fields:

- `tenant` — assigns the RGW user to a named RGW tenant, enabling bucket name isolation across tenants.
- `defaultPlacement` — sets the user's default bucket placement target, controlling which data/metadata pools newly created buckets land in.

Both fields already exist in the underlying `admin.User` struct in `go-ceph`; this work wires them into the Rook controller and API.

## Motivation

### Tenant Isolation

Ceph RGW supports a multitenancy model where users live in named tenants. Users in different tenants can own buckets with the same name without collision:

```
# Two separate objects, no conflict
tenantA$user1 → s3://photos
tenantB$user1 → s3://photos
```

Rook currently has no mechanism to place a `CephObjectStoreUser` in an RGW tenant. Operators who need per-tenant user isolation must manage RGW users manually outside of Rook, forgoing the benefits of the operator (secret rotation, lifecycle management, status conditions).

### Placement Targeting

`CephObjectStore.spec.sharedPools.poolPlacements` already allows defining named placement targets (each backed by distinct metadata/data pools). However, the `CephObjectStoreUser` controller has no way to assign a user's `default-placement`, meaning all users default to the store-wide default placement. For multi-tier storage scenarios (hot/cold pools, per-tenant pools), operators need per-user placement control.

## Goals

- Add `spec.tenant` to `CephObjectStoreUser` to set the `Tenant` field on the RGW Admin Ops API `User` struct at user creation.
- Add `spec.defaultPlacement` to `CephObjectStoreUser` to set `DefaultPlacement` on the user via `CreateUser`/`ModifyUser` in the RGW Admin Ops API.
- Validate `defaultPlacement` against the named placements defined in the referenced `CephObjectStore`.
- Treat `tenant` as immutable (changing tenant requires user deletion and recreation in RGW).
- Treat `defaultPlacement` as mutable (can be updated via `ModifyUser`).
- Both fields are independent of each other: `defaultPlacement` may be set without `tenant`, and vice versa.
- Preserve backward compatibility: both fields are optional, existing resources are unaffected.


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
    ID              string `json:"user_id" url:"uid"`
    Tenant          string `url:"tenant"`           // ← passed as URL param only, not in JSON response
    DefaultPlacement string `json:"default_placement" url:"default-placement"`
    // ...
}
```

**Note**: `Tenant` is URL-parameter-only (no `json` tag) in go-ceph, meaning it is passed in API requests but not present in JSON responses. The controller must store the tenant from spec, not from API responses.

### Interaction with `AccountRef`

`AccountRef` (added in a recent release) also links users to an RGW account and is already marked immutable. Users with `accountRef` set are account-member users. Tenant assignment and account membership are orthogonal in RGW—a user can belong to both a tenant and an account.

## Proposed API Changes

### `ObjectStoreUserSpec` (`pkg/apis/ceph.rook.io/v1/types.go`)

```go
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

    // Placement controls the user's default bucket placement target and storage class tags.
    // May be set independently of tenant.
    // +optional
    // +nullable
    Placement *ObjectStoreUserPlacementSpec `json:"placement,omitempty"`
}

// ObjectStoreUserPlacementSpec sets the user's default placement target and storage class tags.
type ObjectStoreUserPlacementSpec struct {
    // ID names the placement target for new buckets created by this user.
    // Must match one of the entries in the referenced CephObjectStore's
    // spec.sharedPools.poolPlacements[].name.
    // +optional
    // +kubebuilder:validation:Pattern=`^[a-zA-Z0-9._/-]+$`
    // +kubebuilder:validation:MaxLength=255
    ID string `json:"id,omitempty"`

    // Tags is a list of storage class tags to associate with this user's default placement.
    // +optional
    // +listType=atomic
    Tags []string `json:"tags,omitempty"`
}
```

Maps to `go-ceph` `admin.User` fields:
- `Placement.ID` → `DefaultPlacement` (URL param `default-placement`)
- `Placement.Tags` → `PlacementTags` (JSON `placement_tags`)

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
  placement:
    id: hot-tier
    tags:
      - tenant_a_tag
```

## S3 Client Configuration for Tenanted Users

RGW exposes tenanted users to S3 clients through their access key / secret key pair — the S3 client itself requires no special modification. Credentials stored in the Rook-managed Kubernetes Secret are functionally identical regardless of whether the user belongs to a tenant.

```yaml
# AWS CLI profile for a tenanted user — identical to a non-tenanted user
[profile tenantA-user1]
aws_access_key_id     = <AccessKey from rook-ceph-object-user-my-store-user1>
aws_secret_access_key = <SecretKey from rook-ceph-object-user-my-store-user1>
```

Bucket names, however, are only unique within a tenant namespace. Two users in different tenants may each own a bucket named `photos` without conflict. From the client's perspective, each accesses their own `photos` bucket using their respective credentials; RGW routes requests to the correct tenant namespace internally.

Cross-tenant access to another tenant's bucket is not possible via standard S3 path/virtual-host addressing — RGW resolves the bucket to the owner's tenant based on the credentials used. Bucket policies and ACLs apply within the same tenant; cross-tenant sharing is out of scope for this feature.

No changes to existing user documentation are required for the S3 endpoint or credential format.

## Immutability

`tenant` is immutable because RGW does not support moving a user between tenants; the only path is deletion and recreation. Attempting to change `tenant` on an existing `CephObjectStoreUser` would silently create a second user in the new tenant while leaving the original orphaned.

**Enforcement mechanism**: CEL `XValidation` rule on the spec field (same pattern as `accountRef`):
```go
// +kubebuilder:validation:XValidation:message="tenant is immutable",rule="self == oldSelf"
```

This gives a clear admission webhook error on any attempted change.

`defaultPlacement` is mutable — RGW supports changing a user's default placement at any time; it only affects future bucket creation, not existing buckets.

## Interaction with `AccountRef`

`tenant` and `accountRef` are orthogonal. Both can be set simultaneously. When both are set:
- `userConfig.Tenant` is set from `spec.tenant`
- `userConfig.AccountID` is set from `spec.accountRef`

No conflict; `admin.User` has both fields.

## Upgrade and Backward Compatibility

- Both new fields are `omitempty`; existing `CephObjectStoreUser` resources without these fields continue to work unchanged.
- No migration of existing RGW users is required or performed.
- The CRD schema is additive only.
