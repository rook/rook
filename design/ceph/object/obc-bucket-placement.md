---
title: bucket placement for ObjectBucketClaim provisioning
target-version: release-1.21
---

# Bucket placement for ObjectBucketClaim provisioning

- **Sibling design**: [RGW User Multitenancy and Default Placement Targeting](../rgw-user-multitenancy-and-placement.md)
    (`CephObjectStoreUser.spec.defaultPlacement` / `defaultStorageClass`)
- **Prototype**: https://github.com/jhoblitt/rook/pull/4 (implementation, unit and
    integration tests; it spells the placement key `locationConstraint`,
    also accepts the keys as StorageClass parameters, and logs instead of
    failing on an existing bucket, see [Alternatives](#alternatives))

## Summary

Buckets provisioned for an `ObjectBucketClaim` (OBC) always land in the RGW
placement target that the bucket's owner defaults to — for the user the
provisioner generates per OBC, that is the zonegroup default. This proposal
lets a claim request the bucket's placement target and default storage class
through two new `spec.additionalConfig` keys:

| key | value | RGW effect |
|---|---|---|
| `bucketPlacement` | a placement-target name, as in `CephObjectStore.spec.sharedPools.poolPlacements[].name` and `CephObjectStoreUser.spec.defaultPlacement` | the bucket's placement target (which pools hold its index and data) |
| `bucketStorageClass` | a storage class of that placement target | the bucket's default storage class, used by objects written without an explicit `x-amz-storage-class` |

Both keys are default-disabled: an administrator enables them by adding
them to `ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS`, as for `bucketPolicy` and
`bucketOwner`, because they let a namespace user choose the tier a bucket
consumes. Each key is applied only when the provisioner creates the bucket;
RGW bucket placement is immutable after creation. A claim that names a
placement its bucket does not have is a wrong spec, and the provisioner
refuses to reconcile it (see [Existing buckets](#existing-buckets)).

### Goals

- Let a claim request a placement target and/or default storage class, on
    every supported Ceph version with identical behavior.
- Leave validation to RGW: the serving RGW validates both values against its
    zonegroup and zone at `CreateBucket`; the operator reports RGW's error and
    performs no lookup against a rook CR (which would be wrong for zone-backed
    and external stores, where the store CR carries no placement list).
- Make a request that RGW cannot honor visible as a reconcile failure, never
    a silent success.
- Compose with the sibling design: a bucket request wins over the owner
    user's default, which wins over the zonegroup default. This is RGW's own
    precedence (`select_bucket_placement`: requested rule > user default >
    zonegroup default), not a rook-side rule.
- No modifications required to lib-bucket-provisioner.

### Non-Goals

- Moving an existing bucket to another placement (RGW cannot).
- Managing placement targets or storage classes; that is
    `CephObjectStore.spec.sharedPools.poolPlacements` or `radosgw-admin`.
- StorageClass parameters for these keys (an administrator default per
    class). How StorageClass parameters and OBC `additionalConfig` relate —
    whether lib-bucket-provisioner should consolidate them, and which wins —
    is a question for every OBC field, not for placement, and is deferred to
    that discussion.
- Authorizing who may use a placement target. RGW already does that: on
    every `CreateBucket` it checks the bucket owner's `placement_tags`
    against the target's tags and answers `AccessDenied` on a mismatch,
    configured with `radosgw-admin` regardless of rook's API (the sibling
    design defers a rook API for tags). Two consequences shape this design:
    the per-OBC generated user carries no tags and rook does not set any, so
    a tagged target is reachable today only through `bucketOwner` naming a
    tagged user; and RGW never authorizes a storage class per user — at
    creation the check is existence-only, and any writer selects a class per
    object with `x-amz-storage-class` — so `bucketStorageClass` is a default,
    not a tier control. A tier that must be per-user authorized is modeled
    as its own placement target. The `additionalConfig` allowlist gates
    which OBC keys may be set, not which targets a user may reach.
- Tenanted bucket owners (`bucketOwner: tenant$user`). The provisioner
    addresses buckets by bare name in the default tenant for its exists
    check, `Grant`, and `Delete`; the comparison below inherits that.
- COSI (`ceph-cosi-driver`) parity.

## Proposal details

### Keys

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-bucket
spec:
  generateBucketName: ceph-bkt
  storageClassName: rook-ceph-bucket
  additionalConfig:
    bucketPlacement: archive        # optional; a placement target of the store
    bucketStorageClass: COLD        # optional; a storage class of that target
```

A key that is present is sent to RGW; a key that is absent or empty sends
nothing, and RGW applies its own precedence. `additionalConfigSpecFromMap`
(`pkg/operator/ceph/object/bucket/util.go`) parses both keys like the
existing ones: a key present in the OBC but not in
`ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS` fails provisioning with the
existing "OBC config is not allowed" error. That allowlist is the only
rook-side control over the keys; it is process-wide and keyed by name, so
enabling a key enables it for every OBC in every namespace.

`bucketStorageClass` does not require `bucketPlacement`: RGW inherits the
placement name from the owner's default (or the zonegroup's) and keeps the
requested class. The reverse does not hold: a request that names a
placement but no class gets `STANDARD`, not the owner's default storage
class — RGW inherits from the owner's default only when the placement name
is empty.

Both values must match `^[a-zA-Z0-9._-]+$`, the pattern the sibling design
puts on `defaultPlacement`. The OBC's `additionalConfig` is an unvalidated
map in lib-bucket-provisioner's CRD, so the provisioner is the only place to
check it, and it rejects other values before any RGW call. `/` is excluded
because it is RGW's separator in the `<placement>/<storage-class>`
placement-rule syntax, which RGW itself applies when it re-reads a bucket's
placement; the pre-existing `PoolPlacementSpec.name` pattern permits `/`, so
a slash-bearing target cannot be requested here. Whitespace is excluded
because the storage class travels as a request header that the client and
RGW trim: a value that differs from a name only by a trailing space would
create the bucket under the name and then fail the comparison below on
every later reconcile.

### Wire mapping

The provisioner creates buckets through the S3 API as the bucket's owner, so
the request rides the two S3 `CreateBucket` inputs RGW reads:

- `bucketPlacement` → `CreateBucketConfiguration.LocationConstraint` =
    `":<placement>"`. RGW's grammar for the constraint is
    `<zonegroup api_name>:<placement>`, with the same semantics as an AWS
    region: the zonegroup is already fixed by the endpoint the request goes
    to — the `CephObjectStore` the OBC's StorageClass names — and the prefix
    only asserts it. RGW rejects a client request whose prefix names any
    other zonegroup (`IllegalLocationConstraintException`, or
    `InvalidLocationConstraint` for an unknown name), and an empty prefix
    skips the assertion. Nothing in the constraint can steer a bucket to a
    different zonegroup, so the provisioner sends the prefix empty and never
    exposes it; the suffix selects a placement target within the store's
    zonegroup.
- `bucketStorageClass` → the `x-amz-storage-class` request header, added
    per call through SDK middleware so it is SigV4-signed. A storage class
    never appears in the constraint: RGW takes the whole post-colon
    substring as the placement name (`placement_rule.init()` has no `/`
    parser on this path), so `fast:COLD` would be read as zonegroup `fast`
    and placement `COLD`, and a `placement/class` suffix fails as
    `InvalidLocationConstraint`.

Both parse paths are identical from v19.2.0 through main, so behavior is the
same on every supported Ceph version and nothing in the operator is
version-conditional. Neither form is documented upstream; the bare
`:<placement>` constraint has been stable since Luminous, and
[tracker 40394](https://tracker.ceph.com/issues/40394) has asked for it to
be documented since 2019.

### Existing buckets

RGW validates the request at `CreateBucket`: an unknown placement target, or
a storage class the zone's placement does not define, fails with
`InvalidLocationConstraint`; a placement the owner's `placement_tags` do not
permit fails with `AccessDenied`. The provisioner fails the reconcile on any
such error and performs no validation of its own beyond the allowlist and
the value pattern. One RGW answer needs a rook-side change: a
`CreateBucket` against a bucket that already exists with a different
placement is answered with `409 BucketAlreadyExists`, which the provisioner
today treats as an idempotent success. Under this design a 409 from the
provisioner's `CreateBucket`, when a placement or class was requested, is a
reconcile error naming them; a same-owner, same-config re-create is answered
with 200 on every supported release, so no idempotent path is lost.

`Provision` and `Grant` are re-entered for every reconcile of an OBC — an
operator restart, an idempotent retry, or any later `additionalConfig`
change (lib-bucket-provisioner has no update hook; a changed OBC simply
re-runs provisioning). When the bucket already exists — a brownfield
`Grant`, a re-provision, or an owner relink (the provisioner relinks any
existing bucket whose owner differs) — its placement cannot change, so the
provisioner checks the request against it instead of applying it:

- Read the bucket's `placement_rule` from admin-ops bucket info. RGW reports
    it as `<placement>` or `<placement>/<storage-class>`; the provisioner
    splits on the first `/`, as RGW's own `rgw_placement_rule::from_str`
    does, and an absent class means `STANDARD`.
- If `bucketPlacement` is set and differs from the reported placement name,
    or `bucketStorageClass` is set and differs from the reported class
    (`STANDARD` when absent), the reconcile fails with an error naming both
    the requested and the actual values, before any relink or policy write.
    A claim naming a placement its bucket does not have is wrong, and the
    operator does not accept spec drift.
- A key that is not set is unmanaged and never compared; an unchanged
    re-provision therefore passes.

At creation the OBC stays `Pending`. On a later change the OBC stays
`Bound`, its Secret and ConfigMap intact, and the operator errors and
retries until the claim is corrected. The check runs ahead of the quota,
policy, and lifecycle writes, so those keys do not converge either until the
placement is corrected.

The OBC's own status cannot carry the error: it is phase-only, and the
`Failed` phase is defined by lib-bucket-provisioner but never set. The
provisioner therefore records a **Warning Event on the OBC** for each of the
three failures this design introduces — a value the provisioner rejects,
RGW's rejection of the requested placement or class at
creation (the 409 included, carrying RGW's message), and the existing-bucket
mismatch — naming the requested and, where known, the actual values, using
an event recorder obtained from the controller manager as the object
controllers already do; this is the first rook code to record an Event on an
OBC. `kubectl describe obc` is the author-visible channel; the operator log
carries the same message at Error level for cluster-side alerting.
Pre-existing provisioning failures stay log-only.

### Compatibility and rollback

Both keys are optional; OBCs that carry neither are unaffected, and no state
written by this feature needs migration — the placement lives in RGW's
bucket info, where it always has. An operator that predates the keys ignores
them: `additionalConfigSpecFromMap` examines only the keys it knows. Rolling
back therefore leaves every bucket where it was placed and stops honoring
the keys for buckets created afterwards, silently, without failing a
reconcile. Rolling forward again, a claim that carried a key while the older
operator ignored it has a bucket on the default placement and fails the
comparison above once the key is honored; the author removes or corrects
the key.

### Risks and mitigation

- **Tier selection by non-administrators.** An OBC author allowed the keys
    can place a bucket on the most expensive tier. Mitigation: the keys are
    default-disabled, and the allowlist is the administrator's decision —
    for the whole operator, since it is not per namespace. Beyond it, RGW
    `placement_tags` fence tagged targets, but only against a tagged owner
    named through `bucketOwner`; untagged targets stay reachable by any
    allowed claim, and a storage class has no gate at any layer.
- **Brownfield surprises.** A claim that sets a key while granting access to
    an existing bucket on another placement fails. That is the intended loud
    failure; omit the key for brownfield access.
- **Prototype rename.** The prototype's `locationConstraint` key is renamed,
    its value grammar narrowed, its StorageClass channel dropped, and its
    log-and-succeed replaced by the failure above; nothing has shipped, so
    there is no compatibility burden.

## Drawbacks

- Two more OBC `additionalConfig` keys extend a mechanism whose authorization
    is coarse (a process-wide allowlist of key names, not of values or
    namespaces).
- The existing-bucket check adds a comparison the provisioner did not have,
    and turns spec drift into reconcile errors where the operator was
    previously permissive.

## Alternatives

- **`locationConstraint`, the S3-ism, instead of `bucketPlacement`**: the
    S3 key name carrying S3's own grammar — the raw `<zonegroup>[:<placement>]`
    string passed to RGW verbatim, as the prototype does. It needs no
    rook-side grammar, and S3-literate users recognize it; it could be used
    instead of `bucketPlacement`. It is not the default because the zonegroup
    half can only match the serving zonegroup or fail, the value is not a
    placement name like every other placement field in rook, and the
    existing-bucket comparison would have to parse the S3 grammar. The S3
    name must not be paired with the placement-name grammar: to RGW a bare
    `LocationConstraint` token names a zonegroup, so `locationConstraint:
    archive` would mean something else to everyone who knows S3. If a raw
    pass-through is ever added, it takes this name.
- **StorageClass parameters as an administrator default**, which the
    prototype implements alongside the OBC keys. Deferred: see Non-Goals.
- **User-level default only** (the sibling design). It cannot reach the user
    the provisioner generates per OBC, which has no CR, and it makes placement
    a property of the user rather than of the claim.
- **Setting the generated user's default placement** with `ModifyUser`
    before `CreateBucket`. Rejected: it mutates user state to express a bucket
    property, and is wrong with `bucketOwner`, where the user is shared.
- **One key carrying `placement/class`.** Rejected: `/` is legal in
    `PoolPlacementSpec.name` and is RGW's own placement-rule separator, so
    the form is ambiguous, and RGW does not split it on the wire.
- **Rook-side validation** against `poolPlacements`. Rejected for the reason
    the sibling design gives: zone-backed and external stores carry no placement
    list in a rook CR, and RGW validates anyway.
- **Log and succeed on an existing bucket** (the prototype's behavior).
    Rejected: see [Existing buckets](#existing-buckets).

## Future work

- StorageClass parameters for these keys, once the lib-bucket-provisioner
    question in Non-Goals is settled.
- Placement tags: a rook API for target tags (`PoolPlacementSpec.tags`,
    deferred by the sibling design) and an administrator-controlled tag list
    for the generated user — the pinned go-ceph already transmits
    `placement-tags` on the user calls the provisioner makes — which
    together would fence tagged targets without `bucketOwner`.

## Open Questions

- Whether reviewers prefer the S3-ism `locationConstraint` — name and
    grammar together, see Alternatives — over `bucketPlacement`.
