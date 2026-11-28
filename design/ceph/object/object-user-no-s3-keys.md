---
title: object-user-no-s3-keys
target-version: release-1.22
---

# CephObjectStoreUser without S3 keys

## Summary

A `CephObjectStoreUser` always ends up with at least one S3 key pair. When `spec.keys` is empty,
RGW generates a key as it creates the user, Rook keeps exactly one key, and writes it to a Secret.
A user that has no keys is treated as an error: the reconcile fails with `no keys set for user`.

RGW has no such requirement. A user without S3 keys still owns buckets, is subject to quota,
belongs to an account, and can be named in a bucket policy. Operators want such users when no
long-lived S3 credential should exist for an identity: the user exists to own buckets, or its
access comes from somewhere else, such as STS with a web identity, or Swift.

This proposal adds one field to `CephObjectStoreUser` that declares the user has no S3 keys. Rook
then creates the user without a key, removes any S3 keys the user already has, and does not keep
a credentials Secret for it.

The request came out of [#18830](https://github.com/rook/rook/issues/18830), where keys were
removed from RGW users by hand because Rook had no way to express this.

### Goals

- Create an RGW user that has no S3 keys.
- Remove every S3 key from an existing user by editing its `CephObjectStoreUser`.
- Leave no credentials Secret behind for a user that has no S3 keys.
- Give a keyless user keys again by declaring them in `spec.keys`.
- Leave every `CephObjectStoreUser` that does not set the field behaving exactly as it does today.

### Non-Goals

- **ObjectBucketClaims and bucket notifications with a keyless owner.** Rook signs in to RGW as
    the bucket owner to create a bucket and to configure its notifications, so neither works when
    the owner has no keys. See [Risks and Mitigation](#risks-and-mitigation).
- **Generating a key for an existing user that has none.** Rook does not do this today and this
    proposal does not add it. See
    [Alternatives](#generating-a-key-when-the-field-is-turned-off).
- **Leaving keys unmanaged.** There is no mode in which Rook creates the user and then ignores its
    keys. A user whose keys are created through the IAM API by its account is not served by this
    proposal.
- **Swift credentials.** Swift keys and subusers are not created, removed, or inspected.
- **Continuous enforcement.** Rook removes keys when it reconciles the user. It does not poll RGW.
- **Changes to `spec.keys`.** How explicitly declared keys are reconciled, and what the Secret
    holds in that mode, is unchanged.
- **Account root users.** `CephObjectStoreAccount` manages those and already has
    `rootUser.skipCreate`.

## Proposal details

### API

A new optional field on `ObjectStoreUserSpec`:

```go
// DisableS3Keys declares that the user has no S3 keys. When true, the user is created without a
// key, every S3 key found on the user is removed each time the user is reconciled, and no Secret
// holding credentials is kept for the user. It cannot be combined with keys.
// Setting it back to false does not generate a key: declare keys to give the user keys again.
// In a multisite realm, keys are removed only in the zone the user is reconciled in.
// +optional
DisableS3Keys *bool `json:"disableS3Keys,omitempty"`
```

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: bucket-owner
  namespace: rook-ceph
spec:
  store: my-store
  disableS3Keys: true
```

| `disableS3Keys` | `spec.keys` | Result |
| --------------- | ----------- | ------ |
| unset or `false` | empty | one of the user's keys, in the Secret; fails if it has none (today's behavior) |
| unset or `false` | set | exactly the declared keys (today's behavior) |
| `true` | empty | no S3 keys, no Secret |
| `true` | set | rejected at admission |

The developer guide asks three things of a new field:

- **Default when unset:** `false`, which is today's behavior.
- **Mutability:** the field can be changed in either direction at any time.
- **Removal:** removing the field is the same as setting it to `false`. Neither restores the keys
    or generates a new one, so a user that was keyless fails to reconcile until it is given keys.
    See [Giving a keyless user keys again](#giving-a-keyless-user-keys-again). The guide expects a
    removed field to restore the default state; this departs from that and needs the
    maintainers' agreement.

`spec.secretTemplate` is accepted alongside `disableS3Keys: true` and has no effect while there is
no Secret. It applies again when the Secret is written.

The description of `keys` says today that the operator generates credentials when none are
provided. It is reworded with this change, because a key is generated only as the user is
created and only when `disableS3Keys` is not set.

#### Validation

One CEL rule on the spec rejects the contradictory combination:

```go
// +kubebuilder:validation:XValidation:message="disableS3Keys cannot be combined with keys",rule="!has(self.disableS3Keys) || !self.disableS3Keys || !has(self.keys) || size(self.keys) == 0"
```

`keys: []` is accepted alongside `disableS3Keys: true`, because an empty list declares no keys.

### Reconcile behavior

#### `disableS3Keys: true`

1. **Creating the user.** RGW's admin ops API generates a key on user creation unless it is told
    not to, so Rook sends `generate-key=false`. The user is created with no keys. Rook already
    sets this parameter explicitly, to `true`, when it creates an account root user.
2. **Removing existing keys.** Rook removes every entry in the user's S3 key list. This is the
    purge that `spec.keys` already performs for keys that are not declared, run with nothing
    declared.
3. **No fallback.** Today, when no keys are declared, Rook keeps the first key the user has and
    fails if there is none. Neither applies here: an empty key list is the desired state.
4. **The Secret.** Rook does not write `rook-ceph-object-user-<store>-<name>`. If that Secret
    exists and the `CephObjectStoreUser` is its controller owner, Rook deletes it after the keys
    have been removed. A Secret of that name that the user does not control is left alone.
5. **Status.** `status.keys` is empty and `status.info` has no `secretName`. The user reaches
    `Ready` as before.
6. **Logging.** When Rook removes keys or deletes the Secret, it logs at the default log level
    how many keys it removed and that the Secret was deleted. Access key IDs are not logged at
    that level. Without this, credentials that Rook did not create would disappear with no record
    of when.

The key list RGW reports for a user holds every S3 key attached to it, however it was made. On
top of the keys Rook created, that includes:

- keys added with `radosgw-admin` or the admin ops API;
- S3 keys that belong to the user's subusers;
- for a user in an account, keys created through the IAM `CreateAccessKey` API.

All of them are removed. RGW reports which subuser a key belongs to, but not who created a key,
so keys an account made through IAM cannot be told apart from keys Rook made. This is the reach
`spec.keys` already has; it is stated here because "no keys" makes it the whole point.

#### When keys are removed

Rook reconciles a `Ready` user when its spec changes and when the operator starts or reloads its
controllers. It does not learn of changes made in RGW. A key added to a keyless user there, by an
account through IAM for example, stays until one of those events, and the user reports `Ready` in
the meantime. `Ready` means the last reconcile found no keys, not that there are none now.

#### `disableS3Keys` changed from `false` to `true`

Applications holding the user's keys lose access as soon as the reconcile removes them, and the
Secret they read is deleted. This is the purpose of the field, and the removed keys cannot be
recovered.

#### Giving a keyless user keys again

Setting the field back to `false`, or removing it, never generates a key for a user that exists.

- **With `spec.keys` set in the same edit:** the declared keys are created. The Secret is
    handled as it is for any user with declared keys; this proposal does not change that.
- **With `spec.keys` empty:** the user is now in the default mode and has no keys, so the
    reconcile fails with `no keys set for user`, as it does today for any such user.

To return a user to the default mode, where Rook keeps one key and publishes it in the Secret:

1. Set `disableS3Keys` to `false` and declare a key in `spec.keys`.
2. Remove `spec.keys`. Rook keeps the key, as it does today when the last declared key is
    removed, and writes it to the Secret.

Adding a key to the user in RGW has the same effect as the first step and needs no edit: a user
whose reconcile failed is retried, at intervals that back off to about seventeen minutes.

A default-mode user that has no keys takes the first key RGW lists for it, which is the one with
the lowest access key ID, whoever created it, and publishes that key in the Secret. That is how
the default mode behaves today. It is restated here because this proposal makes "default mode
with no keys" a state a user can be left in by an edit.

### Multisite

`disableS3Keys` takes effect across a realm only when the `CephObjectStoreUser` is reconciled by
the cluster that hosts the metadata master zone.

RGW forwards user creation to the metadata master, so a user created without a key is keyless in
every zone. It forwards most other changes to a user as well, but not the creation or removal of
a key. When Rook removes keys through a zone that is not the metadata master:

- the keys are removed in that zone only, and still authenticate in every other zone;
- a later change to the user at the metadata master can bring the keys back to that zone. Rook's own
    updates to the user's display name, bucket limit, op mask, placement, or capabilities are
    forwarded and count as such a change;
- the `CephObjectStoreUser` reports `Ready` and its Secret has been deleted regardless.

The same holds in the other direction. A key given to a keyless user through a zone that is not
the metadata master, by `spec.keys` or by hand, exists in that zone only and is lost the next
time the user's metadata is synchronized from the master.

A user that is failing with `no keys set for user` in such a zone, after the field was turned
off or dropped by a downgrade, takes up keys that return from the master in this way and
publishes one in the Secret.

Rook does not detect any of this. Declared keys in `spec.keys` behave the same way today. The
user documentation for the field will say that, in a multisite realm, a keyless user must be
declared, and given keys again, in the metadata master zone's cluster.

### Ceph versions

No version gate is needed. Creating a user with `generate-key=false`, and removing a user's last
key, both work through the admin ops API on every Ceph release Rook supports (Squid and later).
RGW creates keyless users itself, for IAM `CreateUser` and for federated identities.

### Upgrade and Downgrade

- **Upgrade:** no existing resource sets the field, and nothing changes for a resource that does
    not set it.
- **Downgrade of the CRD and the operator:** the older CRD does not have the field, and the API
    server drops it from a stored resource the next time that resource is written. The older
    operator sees a user in the default mode with no keys and fails the reconcile with
    `no keys set for user`. It generates nothing, but it does take up a key that appears on the
    user. After upgrading again, set the field again.
- **Downgrade of the operator only:** the newer CRD still accepts the field, and the older
    operator ignores it. An existing keyless user fails with `no keys set for user` and recovers
    without an edit once the operator is upgraded again. A user created in that state is created
    with a generated key and a Secret, which are removed after the upgrade.
- **Downgrade and bucket claims:** an operator without the guard described under
    [Risks and Mitigation](#risks-and-mitigation) panics when it meets a claim whose owner has
    no keys. Before downgrading to such a release, give keys back to any keyless user that owns
    claims.

### Risks and Mitigation

- **ObjectBucketClaim with `bucketOwner`.** The bucket provisioner signs in to RGW with the
    owner's first key, and it does so on every sync of a claim, including claims that are
    already bound, each time the operator starts. On `master` today those paths index an empty
    key list, which panics and takes the operator down, as #18830 reports for users stripped by
    hand. This field makes that state declarable, so a guard on those paths is a prerequisite.
    [#18858](https://github.com/rook/rook/pull/18858) proposes one for the bucket provisioner.
- **Bucket notifications.** The notification controllers read the bucket owner's first key in the
    same unguarded way, and #18858 does not cover them. There the panic is recovered and logged,
    so a notification on a bucket whose owner became keyless is silently not configured. A guard
    there is part of the prerequisite.
- **Where the guard must be.** The field must not ship in a release that lacks both guards, and
    the guards must be in the previous minor release as well, so that an operator downgrade does
    not meet a keyless owner without them.
- **Turning the field on for a user that owns claims.** Existing buckets and their data are
    unaffected. With the guard in place, every claim the user owns then fails to sync, and:
    - the claim stays `Bound` and its Secret still holds the owner's key pair, which no longer
        works;
    - a new claim naming the user as `bucketOwner` is not provisioned;
    - notifications on its buckets are not configured;
    - deleting a claim that keeps its bucket, which is any claim with reclaim policy `Retain`
        or for a bucket that already existed, does not finish. As #18858 stands, the claim
        stays `Terminating`, and its namespace cannot be deleted, until the owner has keys
        again.

    The documentation for `bucketOwner` and for this field will say they do not combine.
    Provisioning the bucket with the admin ops user and linking it to the owner would lift the
    limit, and is left to a follow-up proposal.
- **Consumers of the Secret.** Anything that reads `rook-ceph-object-user-<store>-<name>` fails
    once it is deleted. The COSI driver is pointed at such a Secret, so the user it runs as must
    keep its keys. Deleting the Secret, instead of leaving one without credentials, makes this
    fail where it is easy to see.
- **Users in an account.** Keys an account creates for the user through IAM are removed at the
    next reconcile. The field is for users that must have no keys, not for users whose keys are
    managed elsewhere.
- **A one-word change removes access.** Setting the field on a user in use is destructive. The CEL
    rule stops the commonest mistake, which is setting it on a user that declares keys.

### Security Considerations

The feature removes long-lived credentials, and removes the Secret that would otherwise hold them
in the user's namespace. Rook never generates a key for a user that exists.

Three limits on what the field guarantees are stated above and belong in the user documentation:

- Keys are removed when the user is reconciled, not continuously.
- In a multisite realm, removal is realm-wide and lasting only when the user is reconciled in the
    metadata master zone's cluster. Elsewhere it is neither.
- Once the field is turned off, a user with no declared keys takes up any key that appears on it
    and publishes it in the Secret.

The only new write to Kubernetes is deleting a Secret, limited to one the `CephObjectStoreUser`
controls. Who may remove a user's keys is unchanged: anyone who can edit the
`CephObjectStoreUser` can already replace all of its keys through `spec.keys`.

### Testing

- **Unit tests**, with the admin ops client the existing user controller tests fake:
    - creating a user sends `generate-key=false`, writes no Secret, and reaches `Ready`;
    - an existing user's keys are all removed;
    - the Secret is deleted when the user controls it and kept when it does not;
    - `status.info` has no `secretName` and `status.keys` is empty;
    - the log of a removal carries the count and no access key ID;
    - `true` to `false` with `spec.keys` set creates the declared keys;
    - `true` to `false` with no keys declared fails with `no keys set for user`.
- **Guard tests**, in whichever change adds each guard: the bucket and notification provisioners
    return an error for an owner with no keys.
- **Validation**, in the integration suite against a real API server, as the `secretTemplate`
    rules are tested: the field with `keys` set is rejected; the field with `keys: []` is
    accepted.
- **Integration test**, extending the existing object user keys test: create a keyless user and
    confirm RGW reports no keys and no Secret exists; switch a user with keys to keyless; return
    it to the default mode by declaring a key and then removing the declaration.

## Drawbacks

- The field does not, on its own, give the reporter of #18830 a bucket claim whose owner has no
    key. It is the piece the maintainers agreed belongs in `CephObjectStoreUser`; the provisioner
    change is separate.
- Removing the field does not undo it. A user is left failing until it is given keys, and
    returning to a Rook-published key takes two edits.
- Two fields now describe a user's keys, with one combination forbidden.
- In a multisite realm the field can report success while the keys still work in other zones.
- Setting the field on a user that owns bucket claims breaks those claims.

## Alternatives

### An enum instead of a boolean

A field such as `s3KeyPolicy` with values for "managed" and "none" could later grow a value for
"leave the keys alone" without a second field. Rook's CRDs have a single version and no
conversion webhook, so a boolean cannot later be replaced: a third behavior would arrive as
another field beside this one, with its own validation rules, as `network.provider` arrived
beside `network.hostNetwork`. The boolean was chosen because one new behavior is being asked for
and no one has asked for the third.

### `keys: []` meaning no keys

`spec.opMask` already distinguishes an absent list from an empty one. Doing the same for `keys`
needs no new field, but removing the last declared key currently leaves the user with one of its
existing keys, and would instead strip the user. That changes the meaning of resources that exist
today.

### Generating a key when the field is turned off

Rook could generate a key for a default-mode user that has none, so that setting the field back
to `false` restores a working user in one edit. Rook cannot tell that transition from a user
whose keys were removed outside Rook, so it would also issue a key, and write it to a Secret, for
every existing user in that state at the first reconcile after upgrade, and for a declared-keyless
user whose field was dropped by a downgrade. Recording the declaration in status would separate
the cases, at the cost of status that cannot be rebuilt from RGW. Failing as today issues nothing
and needs no record.

### Rejecting the edit that turns the field off without keys

A CEL transition rule could reject changing the field from `true` unless `spec.keys` is set in
the same update, so that no single edit leaves a user failing. It would also reject a manifest
that merely drops the field, and it does not cover a field dropped by a downgrade. The behavior
is documented instead, in the field's description and the user documentation.

### Refusing to reconcile outside the metadata master zone

Rook could fail the reconcile when the field is set and the store's zone is not the metadata
master. It would turn the multisite limit into a visible error. Rook has no means today of
learning the zone's role for an external object store: its existing check runs `radosgw-admin`,
and the admin API that reports the period has no binding in go-ceph. `spec.keys` has the same
limit without such a check. The limit is documented instead.

### Handling keyless users in the bucket provisioner

Rejected in #18830: user management belongs in `CephObjectStoreUser`, and an OBC should not
change behavior because its user was modified outside Rook.

### Keeping the Secret, without credentials

The Secret could keep `Endpoint` and drop `AccessKey` and `SecretKey`. Nothing would ever be
deleted, but a consumer would find a Secret with missing keys and fail later and less clearly
than it does on a missing Secret.

## Open Questions

1. **The field name.** `disableS3Keys` says what is turned off. `rootUser.skipCreate` on
    `CephObjectStoreAccount` is the nearest existing name, but this field also removes keys that
    exist, which "skip" does not convey.
2. **Subuser keys.** RGW reports which subuser a key belongs to, so S3 keys of subusers could be
    spared. Rook does not manage subusers, and `spec.keys` does not spare their keys today. The
    proposal removes them, because a subuser key is still an S3 credential for the user.
3. **Deleting a claim whose owner has no keys.** The provisioner uses the owner's key at that
    point only to edit the bucket policy. It could skip that edit and finish the deletion,
    instead of returning an error and leaving the claim `Terminating`. This is for the review of
    the guard to settle.
4. **Key changes in RGW multisite.** RGW forwards most changes to a user to the metadata master
    but not the creation or removal of a key. If it forwarded those too, the multisite limit
    would go away. Whether to raise that with Ceph is open.
