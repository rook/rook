# RGW User Multitenancy and Default Placement Targeting in CephObjectStoreUser

- **Issue**: https://github.com/rook/rook/issues/17274

## Summary

This document proposes extending the `CephObjectStoreUser` CRD with four new spec fields:

- `tenant` — assigns the RGW user to a named RGW tenant, enabling bucket name isolation across tenants.
- `defaultPlacement` — sets the user's default bucket placement target, controlling which data/metadata pools newly created buckets land in.
- `defaultStorageClass` — sets the user's default storage class for objects, applied on top of `defaultPlacement`.
- `defaultPlacementTags` — restricts which zonegroup placement targets the user may create buckets in, based on tags carried by those targets.

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
- Leave `defaultPlacement` validation to RGW: the serving RGW validates it
  against the live zonegroup on every create/modify and rejects unknown
  targets with `EINVAL`; the operator surfaces that failure in the CR status.
  (Validating against the referenced `CephObjectStore`'s `spec.sharedPools`
  would be wrong for zone-backed stores, whose placements live on
  `CephObjectZone`, and impossible for external stores.)
- Require `defaultPlacement` to be set whenever `defaultStorageClass` is set, since RGW cannot apply a storage class without a placement target.
- Treat `tenant` as immutable (changing tenant requires user deletion and recreation in RGW).
- Treat `defaultPlacement`, `defaultStorageClass`, and `defaultPlacementTags` as changeable but not removable: they can be updated to a different value via `ModifyUser`, but cannot be unset once set, because RGW provides no way to clear them (see [Unset/Removal Behavior](#unsetremoval-behavior)). This caveat is enforced by CEL and documented for users; lifting it depends on upstream RGW and go-ceph fixes.
- All four fields are independent of `tenant`: `defaultPlacement`/`defaultStorageClass`/`defaultPlacementTags` may be set without `tenant`, and vice versa.
- Preserve backward compatibility: all new fields are optional, existing resources are unaffected.


## Background

### RGW Tenant User ID Format

When a user is created in a tenant, the user's identity everywhere in RGW is the combined form `<tenant>$<uid>`. Every Admin Ops operation resolves the `uid` parameter through this form (`rgw_user::from_str` splits on `$`); a bare `uid` addresses the user in the default (empty) tenant.

The Admin Ops API accepts a separate `tenant` parameter **only on user create**. User info, modify, and remove have no tenant parameter — on those operations a tenant can only be expressed inside the combined `uid`. go-ceph mirrors this: `admin.User.Tenant` is transmitted by `CreateUser` only and silently dropped by `GetUser`/`ModifyUser`/`RemoveUser`, whose URL-parameter whitelists exclude `tenant`.

The controller therefore uses the combined `<tenant>$<name>` string as the user ID for **every** Admin Ops call, create included, and never relies on the go-ceph `Tenant` struct field. Addressing a tenanted user by bare `uid` would silently resolve to the same-named user in the default tenant: reconciliation would adopt (and CR deletion would delete) an unrelated user, or wedge in a CreateUser/UserAlreadyExists loop when none exists. As a backstop, the reconcile fails with an explicit error if the live user's tenant does not match `spec.tenant`, rather than adopting a user from another tenant.

Equivalently via CLI:
```bash
radosgw-admin user create --uid="tenantA$user1" --display-name="User 1"

radosgw-admin user info --uid="tenantA$user1"
```

The `go-ceph` `admin.User` struct models all four fields. Released support
arrived in two steps: v0.40.0 (Rook's current pin) transmits
`default-placement` and `default-storage-class` on both `CreateUser` and
`ModifyUser`; v0.41.0 (released 2026-08-11) adds `placement-tags`
([ceph/go-ceph#1290](https://github.com/ceph/go-ceph/pull/1290)):

```go
// go-ceph v0.41.0 (released)
type User struct {
    ID                  string   `json:"user_id" url:"uid"`
    Tenant              string   `url:"tenant"`  // ← URL param on user create only
    DefaultPlacement    string   `json:"default_placement" url:"default-placement"`
    DefaultStorageClass string   `json:"default_storage_class" url:"default-storage-class"`
    PlacementTags       []string `json:"placement_tags" url:"placement-tags"`
    // ...
}
```

Implementing `spec.defaultPlacementTags` therefore requires only a `go.mod`
bump to go-ceph v0.41.0 — no fork and no unreleased dependency. See
[Setting the storage class](#setting-the-storage-class) for how the two
placement parameters are applied on the wire.


### Interaction with `AccountRef`

`AccountRef` (added in a recent release) also links users to an RGW account and is already marked immutable. Users with `accountRef` set are account-member users. RGW itself allows a tenanted account member as long as the member's tenant equals the account's own tenant, but Rook's `CephObjectStoreAccount` cannot create a tenanted account today — see [Interaction with `AccountRef`](#interaction-with-accountref-1) for why this design rejects the combination at admission rather than exposing an always-failing configuration.

## Proposed API Changes

### `ObjectStoreUserSpec` (`pkg/apis/ceph.rook.io/v1/types.go`)

`defaultPlacement` and `defaultStorageClass` are added as flat, top-level `*string` fields directly on `ObjectStoreUserSpec`, named to match the `go-ceph` `admin.User` fields exactly (`DefaultPlacement`, `DefaultStorageClass`), rather than being wrapped in a nested `ObjectStoreUserPlacementSpec` struct. This mirrors the pattern already established by PR [#17260](https://github.com/rook/rook/pull/17260), which added `DefaultStorageClass` as a flat field on the same spec — introducing a nested struct here would create two incompatible shapes for closely related fields on the same object.

A nested `spec.placement` struct would make the `defaultStorageClass` requires `defaultPlacement` coupling structurally visible, which is the strongest argument for it. The flat layout expresses the same constraint with a CEL rule instead, which produces a specific, actionable error message at admission and does not require users to learn a second nesting level for what is, on the RGW side, three sibling attributes of one user. The coupling is also not total — `defaultPlacement` and `defaultPlacementTags` are each usable on their own — so a wrapper struct would group fields more tightly than their actual semantics warrant.

```go
// ObjectStoreUserSpec represent the spec of an Objectstoreuser
// +kubebuilder:validation:XValidation:message="defaultStorageClass requires defaultPlacement",rule="!has(self.defaultStorageClass) || has(self.defaultPlacement)"
// +kubebuilder:validation:XValidation:message="defaultPlacement cannot be unset once set; RGW does not support clearing a user's default placement",rule="!has(oldSelf.defaultPlacement) || has(self.defaultPlacement)"
// +kubebuilder:validation:XValidation:message="defaultStorageClass cannot be unset once set; RGW does not support clearing a user's default storage class",rule="!has(oldSelf.defaultStorageClass) || has(self.defaultStorageClass)"
// +kubebuilder:validation:XValidation:message="defaultPlacementTags cannot be unset once set; RGW does not support clearing a user's placement tags",rule="!has(oldSelf.defaultPlacementTags) || has(self.defaultPlacementTags)"
// +kubebuilder:validation:XValidation:message="tenant is immutable",rule="has(oldSelf.tenant) == has(self.tenant) && (!has(self.tenant) || self.tenant == oldSelf.tenant)"
type ObjectStoreUserSpec struct {
    Store        string `json:"store,omitempty"`
    DisplayName  string `json:"displayName,omitempty"`
    // ... existing fields ...

    // Tenant is the RGW tenant this user belongs to.
    // Users in different tenants can have buckets with the same name without conflict.
    // When set, the effective user ID in RGW becomes "<tenant>$<name>".
    // This field is immutable after creation.
    // +optional
    // +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_]+$`
    // +kubebuilder:validation:MaxLength=255
    Tenant string `json:"tenant,omitempty"`

    // DefaultPlacement overrides the default pool placement for buckets created by
    // this user. It must name a placement target known to the zonegroup serving
    // the referenced object store; RGW validates this on every create and modify
    // and rejects unknown targets. If not provided, the zone group's
    // default placement target is used.
    // This field cannot be unset once set — see "Unset/Removal Behavior".
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=2048
    // +kubebuilder:validation:Pattern=`^[a-zA-Z0-9._-]+$`
    DefaultPlacement *string `json:"defaultPlacement,omitempty"`

    // DefaultStorageClass overrides the default storage class for objects created by
    // this user. Requires DefaultPlacement to be set. If not provided, the default
    // `STANDARD` storage class is used.
    // This field cannot be unset once set — see "Unset/Removal Behavior".
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=2048
    DefaultStorageClass *string `json:"defaultStorageClass,omitempty"`

    // DefaultPlacementTags restricts which placement targets this user may
    // create buckets in: if a zonegroup placement target carries tags, RGW
    // allows bucket creation there only when one of the user's placement
    // tags matches. Targets without tags remain usable by every user. The
    // tags are not storage-class related and are not scoped to DefaultPlacement.
    // This field cannot be unset once set — see "Unset/Removal Behavior".
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=64
    DefaultPlacementTags []string `json:"defaultPlacementTags,omitempty"`
}
```

Maps to `go-ceph` `admin.User` fields:
- `DefaultPlacement` → `DefaultPlacement` (URL param `default-placement`)
- `DefaultStorageClass` → `DefaultStorageClass` (URL param `default-storage-class`)
- `DefaultPlacementTags` → `PlacementTags` (URL param `placement-tags`, JSON `placement_tags`)

### Setting the storage class

RGW stores a user's default placement as a single `rgw_placement_rule` value
with two members, `name` and `storage_class` (`src/rgw/rgw_placement_types.h`),
but it does not present that value as one field on either side of the admin ops
API:

- **Reading**, the two members are split across two JSON keys —
  `"default_placement"` and `"default_storage_class"` in `RGWUserInfo::dump`
  (`src/rgw/rgw_common.cc`). On squid and tentacle the raw `storage_class`
  member is dumped, so a user that has never had one set reports
  `"default_storage_class": ""`; only on main (v21) does `get_storage_class()`
  canonicalize it to `STANDARD`. The controller treats `""` and `STANDARD`
  as equivalent when comparing desired and live state.
- **Writing**, the wire encoding differs by Ceph version, and the controller
  selects it internally by `cephver` so the CRD contract is identical on
  every supported release:
  - **Tentacle (v20) and later** ([ceph#57985](https://github.com/ceph/ceph/pull/57985),
    [tracker 66439](https://tracker.ceph.com/issues/66439), never backported
    to squid): two separate URL params, and RGW composes the rule itself —
    `target_rule.name = default_placement_str`, then
    `target_rule.storage_class = default_storage_class_str` if non-empty
    (`RGWOp_User_Modify::execute`).
  - **Squid (v19)**: `default-storage-class` is never parsed; the handler
    builds the rule with `target_rule.from_str(default_placement_str)`, which
    splits on the first `/`. On squid only, the controller composes
    `<placement>/<storage-class>` into the `default-placement` parameter — an
    internal, explicitly temporary encoding, removed when squid leaves Rook's
    support window.

  Both encodings produce the identical `rgw_placement_rule` on the user, are
  validated by the same server-side `valid_placement` check (an unknown
  placement or storage class fails with `EINVAL` on both), and are reported
  back identically by user info — so reconcile comparisons, status, and the
  CRD contract are version-blind. Unit tests assert the exact wire fields
  `generateUserConfig` produces for each version.

The `<placement>/<storage-class>` form that appears elsewhere in Ceph is the
*serialized* representation of that struct — `rgw_placement_rule::to_str()`
emits it for on-disk encoding, and `from_str()` splits it back apart on decode.
It is not the admin ops wire format. **Packing a storage class into
`spec.defaultPlacement` (e.g. `hot-tier/STANDARD_IA`) is therefore not
supported, and this design disallows it.** On Tentacle and later, RGW assigns
the parameter to `target_rule.name` verbatim without splitting on `/`, and then
validates it with
`RGWZoneParams::valid_placement`, which does a `placement_pools.find(rule.name)`
lookup (`src/rgw/rgw_zone.h`). A packed string will not match any configured
placement target, so the request is rejected with `EINVAL`. The no-`/`
pattern on `spec.defaultPlacement` additionally rejects a packed value at
admission, before it ever reaches RGW.

So the two Rook fields map one-to-one onto the two URL params, and the operator
sends them separately. Users never see or construct the `/`-joined form. The
reason this needed clarifying is historical: because released `go-ceph` cannot
send `default-storage-class` at all (see [Background](#rgw-tenant-user-id-format)),
packing looked like the only available route to set a storage class — as hit in
[#17260](https://github.com/rook/rook/pull/17260). Fixing it in `go-ceph` is the
correct layer, which is what [ceph/go-ceph#1290](https://github.com/ceph/go-ceph/pull/1290)
does; hiding the joined string from Rook's API is deliberate, since it is an
RGW-internal encoding detail rather than something users should have to know.

**`PlacementTags` availability and scope:** [ceph/go-ceph#1290](https://github.com/ceph/go-ceph/pull/1290) added placement-tags support to the admin ops client and is included in released go-ceph v0.41.0, so this field only needs a `go.mod` bump — no fork. One caveat belongs on record: placement tags act only against zonegroup placement targets that themselves carry tags, and Rook's `PoolPlacementSpec` (on both `CephObjectStore` and `CephObjectZone`) has no `tags` field today. In a fully Rook-managed topology this knob therefore has no effect until targets are tagged out-of-band (`radosgw-admin zonegroup placement modify --tags ...`, which Rook's zonegroup reconcile preserves) or a `PoolPlacementSpec.tags` counterpart lands as follow-up work; deployments with externally managed zonegroups can use it as-is.

### Unset/Removal Behavior

Rook's long-term direction for typed APIs is that unset means disabled: removing
a field from the spec should drive the resource back to its default, the same
way a native Kubernetes API behaves. RGW cannot express that today for these
three fields. The admin ops user-modify handler acts on `default-placement`,
`default-storage-class`, and `placement-tags` only when the submitted value is
non-empty (`if (!default_placement_str.empty())` in
`src/rgw/driver/rados/rgw_rest_user.cc`, `RGWOp_User_Modify::execute` — the same
guard is present on squid, tentacle, and main), so an explicitly empty value is
silently ignored rather than treated as a clear. `radosgw-admin` gates the same
way (`if (!placement_id.empty())` / `if (!tags.empty())` in
`src/rgw/radosgw-admin/radosgw-admin.cc`), so this is a property of RGW itself
and not of the admin ops REST layer. Booleans on the same handler (`suspended`, `system`,
`account-root`) do support this, via an `s->info.args.exists(...)` check that
distinguishes "parameter absent" from "parameter present and empty" — the
placement parameters simply have not adopted that idiom. The client side has the
same gap independently: released `go-ceph` never transmits empty values for
these parameters, so the clear is not expressible from Rook even once RGW
accepts it. Two upstream issues track closing this —
[tracker.ceph.com#79090](https://tracker.ceph.com/issues/79090) (extend the
`exists()` idiom to the placement parameters) and
[ceph/go-ceph#1307](https://github.com/ceph/go-ceph/issues/1307) (send
explicitly empty values). Until both land in versions Rook can require, there is
no working pathway for "revert on field removal."

**Caveat for the initial implementation:** `defaultPlacement`,
`defaultStorageClass`, and `defaultPlacementTags` may be set and may be changed
to another value, but cannot be unset once set. Rather than accept a removal and
silently leave stale state on the RGW user, the CRD rejects the removal outright
with the CEL transition rules shown above, so the failure is a clear, immediate
validation error instead of drift the user cannot see. `MinLength=1` on the
string fields closes the obvious workaround of setting the field to `""`, which
would pass the CEL check and then be ignored by RGW. Users who need to return a
user to the zone group's default placement must delete and recreate the
`CephObjectStoreUser`. This caveat must be stated in the user-facing CRD
documentation as well as in the field's Go doc comment. When the upstream fixes
are released, the CEL rules can be relaxed and the controller taught to send the
explicit clear, moving these fields to the unset-means-disabled behavior we
want; because that change only removes a restriction, it is backward compatible.
Whether to additionally mark these fields experimental for the first release is
left to code/doc review.

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

## Status

The controller echoes applied state into the CR status after a successful
reconcile: the effective `default_placement` and `default_storage_class`
read back from user info (current Ceph releases report an unset storage
class as `""`; the controller treats `""` and `STANDARD` as equivalent).
A placement or storage class rejected by RGW's server-side validation
fails the reconcile and surfaces the RGW error in the CR status; this is
the intended validation UX. A tenant mismatch between spec and the live
user is likewise a surfaced reconcile error, never a silent adoption.

## Multisite

RGW user metadata — including `default_placement` and
`default_storage_class` — is realm-scoped and replicates to every zone via
metadata sync. Placement *targets*, however, are zonegroup-scoped, and
their pools are zone-local. Consequences this design accepts:

- Validation happens at apply time, by the RGW serving the referenced
  object store, against **its** zonegroup only.
- In a realm with multiple zonegroups (or independently managed zone
  specs), a user's synced `default_placement` may name a target that does
  not exist in a peer zonegroup. Bucket creation there fails with
  `InvalidLocationConstraint` at the S3 layer; Rook does not detect this.
  Deployments using per-user placement across zonegroups should define the
  same placement target names in every zonegroup of the realm.
- Rook does not re-validate user placements when zonegroup placement
  targets change after the fact.

Zone-backed stores (`spec.zone.name` set) and external-mode stores are
fully supported: because validation is RGW-side, no rook CR needs to carry
the placement list.

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

## Compatibility and rollback

All fields are optional; CRs created by older Rook are unaffected, and the
new schema invalidates no stored object.

Rolling back to a Rook release that predates `spec.tenant` while tenanted
CRs exist is **destructive**: the older operator addresses the user by
bare name, fails to find the tenanted user, creates an untenanted user
with the same name, and repoints the CR's Secret at it — orphaning the
tenanted user and its buckets. Before downgrading, tenanted
`CephObjectStoreUser` CRs must be removed (or the operator scaled down).
This warning ships in the release notes. The placement fields carry no
such hazard: an older operator simply leaves the live values in place.

## Immutability

`tenant` is immutable because RGW does not support moving a user between
tenants; the only path is deletion and recreation. Attempting to change
`tenant` on an existing `CephObjectStoreUser` would silently create a second
user in the new tenant while leaving the original orphaned.


`defaultPlacement`, `defaultStorageClass`, and `defaultPlacementTags` are
mutable — RGW supports changing a user's default placement, storage class, and
placement tags at any time; changes only affect future bucket/object creation,
not existing buckets/objects. Removal is the exception: as described in
[Unset/Removal Behavior](#unsetremoval-behavior), RGW offers no way to clear
these values, so the CRD rejects an update that unsets a previously set field.
This is a caveat of the initial implementation, not the intended end state.

## Interaction with `AccountRef`

RGW requires an account member's tenant to equal the account's tenant
(`validate_account_tenant`, enforced at user create and modify). Rook's
`CephObjectStoreAccount` cannot currently create tenanted accounts, so
every expressible `tenant` + `accountRef` combination would fail at RGW
with `EINVAL` on a doubly-immutable field pair, permanently wedging the
CR. The spec therefore rejects the combination at admission:

```go
// +kubebuilder:validation:XValidation:message="tenant cannot be combined with accountRef (CephObjectStoreAccount does not support tenants)",rule="!(has(self.tenant) && has(self.accountRef))"
```

Supporting tenanted account members later requires a tenant field on
`CephObjectStoreAccount` (go-ceph already transmits one on account
create/modify) and lifting this rule — a backward-compatible relaxation.
