# RGW User Multitenancy and Default Placement Targeting in CephObjectStoreUser

- **Issue**: https://github.com/rook/rook/issues/17274

## Summary

This document proposes extending the `CephObjectStoreUser` CRD with three new spec fields:

- `tenant` — assigns the RGW user to a named RGW tenant, enabling bucket name isolation across tenants.
- `defaultPlacement` — sets the user's default bucket placement target, controlling which data/metadata pools newly created buckets land in.
- `defaultStorageClass` — sets the user's default storage class for objects, applied on top of `defaultPlacement`.

All three fields already exist in the underlying `admin.User` struct in `go-ceph`; this work wires them into the Rook controller and API.

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
- Add `spec.defaultStorageClass` to `CephObjectStoreUser` to set `DefaultStorageClass` on the user, embedded into the placement rule sent to RGW (see [Proposed API Changes](#proposed-api-changes)).
- Validate `defaultPlacement` against the named placements defined in the referenced `CephObjectStore`.
- Require `defaultPlacement` to be set whenever `defaultStorageClass` is set, since RGW cannot apply a storage class without a placement target.
- Treat `tenant` as immutable (changing tenant requires user deletion and recreation in RGW).
- Treat `defaultPlacement` and `defaultStorageClass` as mutable (can be updated via `ModifyUser`).
- Explicitly reconcile removal of `defaultPlacement`/`defaultStorageClass` back to Ceph's defaults rather than leaving the RGW-side value stale (see [Unset/Removal Behavior](#unsetremoval-behavior)).
- All three fields are independent of `tenant`: `defaultPlacement`/`defaultStorageClass` may be set without `tenant`, and vice versa.
- Preserve backward compatibility: all new fields are optional, existing resources are unaffected.
- Mirror the flat-field, go-ceph-aligned API shape and controller approach (`generateUserConfig`/`isUserSync`/`effectiveDefaultPlacement`) already established by PR [#17260](https://github.com/rook/rook/pull/17260) (`DefaultStorageClass`), rather than introducing a parallel, differently-shaped design.


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
    PlacementTags       []string `json:"placement_tags" url:"placement-tags"`
    // ...
}
```

**Note**: `Tenant` is URL-parameter-only (no `json` tag) in go-ceph, meaning it is passed in API requests but not present in JSON responses. The controller must store the tenant from spec, not from API responses.

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
}
```

Maps to `go-ceph` `admin.User` fields:
- `DefaultPlacement` → `DefaultPlacement` (URL param `default-placement`, embeds storage class — see below)
- `DefaultStorageClass` → `DefaultStorageClass` (URL param `default-storage-class`, informational only — see below)

**`PlacementTags` is explicitly out of scope for this design.** PR #17260 does not implement it, and adding it here would reintroduce exactly the kind of parallel, inconsistent surface this revision is trying to eliminate; it can be added later as its own flat `DefaultPlacementTags []string` field once a concrete use case and matching go-ceph wiring exist.

### `DefaultStorageClass` / `DefaultPlacement` Interaction

RGW's handling of the default storage class is unusual: on Squid, the Admin Ops API's separate `default-storage-class` request parameter is ignored. The storage class only takes effect when embedded directly in the placement rule sent to RGW, as `<placement>/<storage-class>`. `User.DefaultStorageClass` in the JSON response is populated from that embedded value, but is not itself an input RGW acts on.

Consequently:
- `defaultStorageClass` **requires** `defaultPlacement` to also be set. This is enforced by a CEL `XValidation` rule on `ObjectStoreUserSpec` (`!has(self.defaultStorageClass) || has(self.defaultPlacement)`), identical to the rule added in PR #17260.
- The controller must not send `defaultStorageClass` to RGW as a bare `default-storage-class` parameter and expect it to take effect. It must build the outgoing `DefaultPlacement` value as `"<defaultPlacement>/<defaultStorageClass>"` before calling `CreateUser`/`ModifyUser`, exactly as PR #17260's `generateUserConfig` does.
- Because RGW reports the storage class back as a separate `DefaultStorageClass` field while Rook writes it embedded in the placement string, comparing live vs. desired state naively would never converge. This design reuses PR #17260's `effectiveDefaultPlacement` helper (splits `"<placement>/<storage-class>"` back into its two parts) inside `isUserSync`, rather than reimplementing this comparison.

This doc's controller should replicate PR #17260's `generateUserConfig`/`isUserSync`/`effectiveDefaultPlacement` approach directly rather than reinventing an equivalent.

### Unset/Removal Behavior

Ceph RGW's `ModifyUser` semantics do not treat an absent field as "no change" — the field's zero value is sent, and RGW acts on it:

- If `defaultPlacement` is set and later removed from the spec, the controller must issue a `ModifyUser` call with an explicit empty `default-placement`, which reverts the user to the zone group's default placement target. Simply omitting the field from a subsequent reconcile is not sufficient — go-ceph's `ModifyUser` sends whatever value is present on the `admin.User` struct passed to it, so a removed spec field must be explicitly reconciled by the controller (e.g. by constructing the request with `DefaultPlacement: ""`), not just left unset in Go.
- Symmetrically, if `defaultStorageClass` is set and later removed while `defaultPlacement` remains set, the controller must send the placement rule without the embedded storage class suffix (i.e. plain `<defaultPlacement>`, not `<defaultPlacement>/<defaultStorageClass>`), which reverts the user to Ceph's `STANDARD` storage class.
- If both are removed together, the controller reverts to sending an empty `default-placement`, which implicitly also clears any storage class override.

This mirrors the enforcement mechanism already used for `tenant`'s immutability: the desired end-state is expressed declaratively in the spec, and the controller is responsible for issuing whatever explicit RGW admin API call is needed to converge live state to it, including reverting fields to their Ceph-side defaults on removal — not merely for what to send when a field is populated.

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

This gives a clear error on any attempted change.

`defaultPlacement` and `defaultStorageClass` are mutable — RGW supports changing a user's default placement and storage class at any time; changes only affect future bucket/object creation, not existing buckets/objects. As described in [Unset/Removal Behavior](#unsetremoval-behavior), mutability also covers the removal case: the controller must actively drive RGW back to its own defaults rather than treating an unset field as a no-op.

## Interaction with `AccountRef`

`tenant` and `accountRef` are orthogonal. Both can be set simultaneously. When both are set:
- `userConfig.Tenant` is set from `spec.tenant`
- `userConfig.AccountID` is set from `spec.accountRef`

No conflict; `admin.User` has both fields.

## Upgrade and Backward Compatibility

- All new fields (`tenant`, `defaultPlacement`, `defaultStorageClass`) are `omitempty`; existing `CephObjectStoreUser` resources without these fields continue to work unchanged.
- No migration of existing RGW users is required or performed.
- The CRD schema is additive only, and reuses the same flat-field, CEL-validated shape introduced by PR #17260 for `defaultStorageClass`, so the two proposals compose without conflicting field layouts.
