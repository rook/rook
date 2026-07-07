---
title: Bucket versioning support for ObjectBucketClaims
target-version: release-1.21
---

# Bucket versioning support for ObjectBucketClaims

Reference issue: [#17318](https://github.com/rook/rook/issues/17318)

## Summary

[S3 bucket versioning](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Versioning.html)
keeps multiple variants of an object in the same bucket, allowing users to
preserve, retrieve, and restore every version of every object stored in a
bucket. Ceph RGW supports the standard S3 `GetBucketVersioning` /
`PutBucketVersioning` APIs.

Today, users who provision buckets via ObjectBucketClaims (OBCs) must enable
versioning out-of-band with an S3 client after the bucket is provisioned. This
proposal adds a `bucketVersioning` key to the OBC `spec.additionalConfig` map
so that versioning is declaratively managed by the Rook bucket provisioner,
following the same pattern as the existing `bucketPolicy` and
`bucketLifecycle` config fields.

## Versioning has three states, not two

A boolean is not sufficient to model S3 versioning. Per the
[S3 documentation](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Versioning.html#versioning-states),
a bucket is always in exactly one of three versioning states:

1. **Unversioned** (the default): the bucket has never had versioning enabled.
   Objects have a `null` version ID.
2. **Versioning-enabled** (`Enabled`): all new objects get a unique version ID.
3. **Versioning-suspended** (`Suspended`): versioning was previously enabled
   and is now suspended. Existing object versions are retained; new objects
   get a `null` version ID.

Critically, the transition out of the unversioned state is **one-way**: once
versioning has been enabled on a bucket, the bucket can never return to the
unversioned state — it can only alternate between `Enabled` and `Suspended`.

The proposed `bucketVersioning` value is therefore a string enum rather than a
bool, with values matching the S3 `VersioningConfiguration.Status` wire
values:

| `bucketVersioning` value | Meaning |
| ------------------------ | ------- |
| _key not present_        | Rook does not manage versioning. The bucket's current versioning state, whatever it is, is left untouched. |
| `Enabled`                | Rook reconciles the bucket's versioning status to `Enabled`. |
| `Suspended`              | Rook reconciles the bucket's versioning status to `Suspended`. |

Any other value is a validation error, and the OBC will fail to reconcile with
an error event, consistent with how invalid `maxSize`/`maxObjects` values are
handled. Validation is case-sensitive: only the exact strings `Enabled` and
`Suspended` (matching the S3 `VersioningConfiguration.Status` wire values) are
accepted. Variants such as `enabled`, `ENABLED`, or `suspended` are rejected.

### Why "key not present" means "unmanaged" instead of "delete"

For `bucketPolicy` and `bucketLifecycle`, removing the key from
`additionalConfig` causes the provisioner to delete the live policy/lifecycle
configuration. Versioning cannot follow that convention because there is no
"delete" operation for versioning state — an S3 bucket that has ever been
versioned cannot go back to unversioned.

The two candidate semantics for a removed `bucketVersioning` key are:

1. **Unmanaged** (proposed): leave the live state as-is.
2. **Suspend**: reconcile the bucket to `Suspended`.

Option 2 is rejected because it makes key-removal a destructive state change
(new object writes silently stop being versioned), which is surprising and
inconsistent with the general principle that removing a config key returns
control to the user rather than forcing a specific state. Option 1 also
matches what `Suspended` is for: users who explicitly want versioning off
can say so.

This asymmetry with `bucketPolicy`/`bucketLifecycle` removal semantics will be
called out in the user-facing documentation.

### `Suspended` on a never-versioned bucket

Calling `PutBucketVersioning` with `Suspended` on an unversioned bucket is
accepted by the S3 API and by RGW; the bucket transitions to the
versioning-suspended state, which behaves equivalently to unversioned for new
writes. The provisioner does not need to special-case this, but the
documentation will note that setting `Suspended` on a fresh bucket is
effectively a no-op for object behavior while still marking the bucket as
"has seen versioning".

## Proposal details

### API

No CRD/API schema changes are required. `ObjectBucketClaim.spec.additionalConfig`
is already an arbitrary `map[string]string` defined by lib-bucket-provisioner.

Example OBC:

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-bucket
spec:
  bucketName: ceph-bucket
  storageClassName: rook-ceph-bucket
  additionalConfig:
    bucketVersioning: "Enabled" # one of "Enabled" or "Suspended"
```

### Operator gating (disabled by default)

Like `bucketMaxObjects`, `bucketMaxSize`, `bucketPolicy`, `bucketLifecycle`,
and `bucketOwner`, the new `bucketVersioning` field will **not** be allowed by
default. Administrators must opt in via the existing
`ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS` operator setting, e.g.:

```
ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS: "bucketVersioning"
```

The risk profile is lower than `bucketPolicy` (a user cannot affect other
users' buckets), but enabling versioning can grow storage consumption without
bound if no lifecycle rule cleans up noncurrent versions, and bucket quota
accounting includes all versions. This is an administrator-visible tradeoff
and justifies keeping the field opt-in. The documentation will recommend
pairing `bucketVersioning` with a `bucketLifecycle` rule using
`NoncurrentVersionExpiration`.

### Provisioner behavior

When `bucketVersioning` is set in the OBC `additionalConfig`, the provisioner
manages the bucket's versioning state during both Provision (greenfield) and
Grant (brownfield) paths. It follows the same read-compare-write pattern as
`bucketPolicy` and `bucketLifecycle`:

1. Read the current versioning configuration from the bucket via
   `GetBucketVersioning` (using the bucket owner's credentials, so this
   composes with `bucketOwner`).
2. If `bucketVersioning` is unset, take no action — the live state is left
   as-is (unmanaged).
3. Compare the live status against the desired status. An unversioned bucket
   (empty live status) is treated as different from the desired
   `Enabled`/`Suspended` value.
4. On difference, call `PutBucketVersioning` with the desired
   `VersioningConfiguration.Status` and log the change.

Versioning is applied **before** lifecycle configuration, so that a
`bucketLifecycle` rule targeting noncurrent versions (e.g.
`NoncurrentVersionExpiration`) has versioned objects to act on.

The operation is idempotent and level-triggered: repeated reconciles converge
and drift introduced by direct S3 API calls is corrected on the next
reconcile, consistent with the other `additionalConfig` fields.

### Greenfield and brownfield

- **Provision** (greenfield): versioning is applied immediately after bucket
  creation and quota setup.
- **Grant** (brownfield, pre-existing bucket): versioning is reconciled on the
  existing bucket the same way quotas are today. If the existing bucket is
  already versioned and the OBC does not set `bucketVersioning`, the live
  state is preserved.

### Out of scope (and how this design leaves room for it)

- **MFA Delete**: `VersioningConfiguration` also carries an `MFADelete`
  field. RGW's support is limited and OBCs have no natural way to carry MFA
  device state. Not supported; the single-key string design leaves room for a
  separate `bucketVersioningMFADelete` key later without breaking changes.
- **Object Lock** ([#17883](https://github.com/rook/rook/issues/17883)):
  object lock requires versioning to be enabled at bucket **creation** time
  (`ObjectLockEnabledForBucket` on `CreateBucket`) and prevents versioning
  from ever being suspended. It is a distinct feature with its own retention
  configuration and should be designed separately. This proposal is a
  prerequisite step in that direction: a future `bucketObjectLock` key would
  compose with `bucketVersioning: Enabled` and add a validation rejecting
  `Suspended` when object lock is requested.
- **CephObjectStoreUser / COSI**: this proposal only covers the OBC
  provisioner. COSI support can reuse the same provisioner-side helper when
  COSI bucket features are extended.

## Validation and testing

- Unit tests for `additionalConfigSpecFromMap()`: accepted values, rejected
  values (`true`, `enabled`, arbitrary strings), and allow-list gating.
- Unit tests for `setBucketVersioning()` using the existing mocked S3/admin
  API test fixtures in `provisioner_test.go`: enable on unversioned bucket,
  no-op when in sync, suspend transition, unmanaged when key absent.
- CI integration: extend the object bucket e2e suite
  (`tests/integration`) with an OBC that sets `bucketVersioning: Enabled`,
  asserts `GetBucketVersioning` returns `Enabled`, flips to `Suspended`, and
  asserts convergence.

## Documentation changes

- `Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md`:
  document the new key, its three-state semantics, the one-way nature of
  enabling versioning, the unmanaged-when-absent behavior, and the
  recommendation to pair with a noncurrent-version lifecycle rule.
- `deploy/examples/object-bucket-claim-versioning.yaml` (or extend the
  existing OBC example): commented example manifest.
