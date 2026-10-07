---
title: raw-mode-host-based-clusters
target-version: release-1.18
---

# Ceph OSD Raw Mode for Host-Based (Node-Local) Clusters

> Related upstream issues:
> [rook/rook#16928](https://github.com/rook/rook/issues/16928) ("Support At-Rest Encryption for
> Host-Based Clusters (raw mode)"), reopened on request, stalled on "what's the key-management
> design" — this document answers that question. [rook/rook#16992](https://github.com/rook/rook/issues/16992)
> reports the same underlying gap (no KMS path for host-based OSDs at all) from the
> `security.kms`/encryption side, closed by the stale-bot without a design.

## Summary

Rook provisions OSDs two ways: `storageClassDeviceSets` (PVC-backed), which uses `ceph-volume raw`,
and node-local devices (`useAllDevices`/`devices:`), which can only use `ceph-volume lvm batch`/`lvm
prepare`. This split is not cosmetic — it means several capabilities that exist in `ceph-volume raw`
today are simply unreachable for host-based clusters, most importantly `security.kms` support (the
same mechanism [key-encryption-key-rotation.md](key-encryption-key-rotation.md) builds on for
PVC-backed OSDs). When `encryptedDevice: true` is set on node-local OSDs, `ceph-volume` self-generates
the dmcrypt key and persists it only to the Ceph monitor's config-key store via the native lockbox
mechanism — there is no code path to an external KMS at all, with no error or condition indicating so.
A cluster with `security.kms` configured and node-local OSDs looks identical, from the CephCluster
status, to one working correctly.

This proposal is to add `ceph-volume raw` as a provisioning option for host-based/node-local OSDs,
primarily to unlock `security.kms` support for them, reusing Rook's existing PVC-mode KMS wiring
pattern rather than inventing a new one. The technical claims below aren't speculative — each one was
verified directly against a running cluster (see the Verified subsections), including a full
prepare-to-activate round trip using a real KMIP server as the actual source of truth for the LUKS
passphrase, with no dependency on the mon's lockbox at any point.

### Goals

- Make `ceph-volume raw` usable as a host-based/node-local OSD provisioning path in Rook.
- Make `security.kms` usable for host-based/node-local OSDs once that path exists, using the same
  configuration surface PVC-backed OSDs already use.
- No behavior change for clusters that don't opt in — node-local OSDs provisioned via `lvm` mode keep
  working exactly as they do today.
- Surface a clear condition/event when `security.kms` is configured but can't be honored (today's
  silent no-op), rather than requiring raw mode support before this gap is even visible to users.

### Non-Goals

- Deprecating or replacing `lvm` mode for host-based clusters. `lvm batch`'s automatic multi-device
  placement (DB/WAL splitting across a shared NVMe, `osdsPerDevice > 1`, consuming pre-existing
  LVs/VGs) has no equivalent in raw mode (see Drawbacks) — this is an additional path, not a
  replacement, at least until/unless that gap is separately closed.
- Migrating the key of an already-provisioned lvm-mode OSD into raw mode or into an external KMS.
  This proposal covers provisioning-time behavior for new OSDs only.
- Extending the `security.keyRotation` CronJob (see
  [key-encryption-key-rotation.md](key-encryption-key-rotation.md)) to host-based OSDs in the same
  change. A reasonable fast-follow once provisioning-time support has run in production.

## Proposal details

### The gap, precisely

1. **Rook** (`pkg/operator/ceph/cluster/osd`): all `security.kms` wiring — key generation in
   `create.go`'s `startProvisioningOverPVCs()` (`GenerateDmCryptKey()`, `kms.NewConfig()`,
   `kmsConfig.PutSecret(osdProps.pvc.ClaimName, key)`), environment/volume injection in
   `provision_spec.go`, and the init container that fetches the key before OSD start in
   `spec.go`'s `getPVCEncryptionOpenInitContainerActivate()` — is gated behind `osdProps.onPVC()`.
   The node-local path, `startProvisioningOverNodes()`, has none of this.
2. **ceph-volume** (upstream Ceph): `ceph_volume/objectstore/raw.py` already reads
   `os.getenv('CEPH_VOLUME_DMCRYPT_SECRET', '')` at both `prepare()` and `_activate()` to accept an
   externally-supplied key. `ceph_volume/objectstore/lvm.py` has no equivalent — the shared
   `BaseObjectStore.__init__` it inherits from unconditionally self-generates the key via
   `encryption_utils.create_dmcrypt_key()`, and nothing in `lvm.py` ever overrides it.

The second point is the important one: the capability this feature needs **already exists and ships
in production today**, in `raw.py`. It's simply unreachable for host-based clusters, because they
can't use raw mode at all today and are confined to `lvm.py`, which never got the equivalent hook.
That reframes the problem: the fastest path to KMS support for host-based clusters is not teaching
`lvm.py` a new trick, it's adding raw mode as a host-based provisioning option and reusing Rook's
existing PVC wiring pattern against it.

### Verified: OSD identity without LVM tags

A natural objection: lvm mode identifies its OSDs via LVM tags (`ceph.osd_fsid`, etc.) — does raw
mode have anything equivalent for host-based (untagged, non-LVM) devices? Tested directly against a
running Rook-managed PVC/raw-mode OSD, encrypted via an existing KMIP integration:

```
# Locked raw device: bluestore label is NOT readable, as expected
$ ceph-bluestore-tool show-label --dev /dev/vdb
No valid bdev label found

# Opened (decrypted) mapper: full label reads fine
$ ceph-bluestore-tool show-label --dev /dev/mapper/<claim>-block-dmcrypt
{ "osd_uuid": "77fe35ef-...", "ceph_fsid": "df5e7fb4-...", "whoami": "0", "type": "bluestore", ... }

# The still-locked device's LUKS2 header itself carries identity metadata,
# readable without any key:
$ cryptsetup luksDump /dev/vdb
Label:      pvc_name=set1-data-05xjnx
Subsystem:  ceph_fsid=df5e7fb4-...
```

The bluestore label genuinely only exists on the decrypted view, never on the locked device — same
constraint lvm mode has. Identity before unlocking comes from the LUKS2 header's own plaintext
metadata fields (`ceph_volume/util/encryption.py`'s `CephLuks2.is_ceph_encrypted`/`get_osd_fsid()`),
the same role an LVM tag plays for lvm mode. One nuance worth being explicit about: Rook's existing
PVC path doesn't exercise this ceph-volume-native mechanism at all — Rook formats and opens the LUKS
container itself, in its own init container, with its own Label/Subsystem convention (confirmed
above: `Subsystem` held the *cluster* fsid, not the OSD fsid `CephLuks2.get_osd_fsid()` expects to
find there) — then hands ceph-volume an already-unlocked device. Rook already owns OSD identity
end-to-end for PVC-mode OSDs; it has no need for ceph-volume's own locked-header discovery. The same
pattern — Rook owning identity assignment rather than depending on ceph-volume's native discovery —
is what the Identifier step below proposes for the host-based path too.

### Verified end-to-end: two OSDs from one split disk, externally-supplied keys

Ran the full prepare-to-activate round trip on a dev cluster: partitioned a spare disk into two
pieces and created two independent raw-mode OSDs, each encrypted with its own externally-supplied key
(simulating what Rook's KMS wiring would inject), to pressure-test both the identity claim above and
the split-one-device-into-multiple-OSDs capability host-based clusters rely on today via
`osdsPerDevice`.

- **`--dmcrypt` without a key source is a hard failure, not a silent fallback.** `ceph-volume raw
  prepare --dmcrypt` with `CEPH_VOLUME_DMCRYPT_SECRET` unset refuses outright: *"encryption was
  requested (--dmcrypt) but environment variable CEPH_VOLUME_DMCRYPT_SECRET is not set ... or use
  --with-tpm"*. There's no self-generate-and-lockbox fallback for raw mode the way lvm mode has —
  raw-mode encryption has always required either an externally-supplied key or TPM. That removes a
  risk this proposal might otherwise have carried (an unset env var silently producing an
  empty-password LUKS volume): it simply can't happen.
- **`ceph-volume raw activate` cannot unlock a cold/locked device by itself.** Setting
  `CEPH_VOLUME_DMCRYPT_SECRET` and running `raw activate --osd-id/--osd-uuid` against a device that
  was never opened this boot fails with *"did not find any matching OSD to activate"* —
  `direct_report()`'s discovery runs `ceph-bluestore-tool show-label` against the device as given, and
  a locked LUKS2 device has no readable bluestore label for it to find. Activation only succeeds once
  the mapper already exists: manually running `cryptsetup luksOpen` with the correct key first, then
  calling `raw activate`, succeeds and reads the label correctly off the resulting
  `/dev/mapper/...` path. **This means the activation step (Step 4 below) must itself perform the
  unlock** — it cannot rely on `ceph-volume raw activate` to do that on its own.
- **Wrong key fails cleanly.** A bogus passphrase against the same LUKS2 header is rejected
  (`cryptsetup`: "No key available with this passphrase.", exit 2) — no silent corruption or
  fallback.
- **The identity finding above holds from a second angle.** Both freshly-created OSDs' LUKS2 headers
  carried `Subsystem: ceph_fsid=<their own osd_uuid>` while still locked, readable via `cryptsetup
  luksDump` with no key — this time set by ceph-volume's own `prepare_dmcrypt()`, unlike the
  Rook-managed PVC OSD examined above, whose Subsystem field held the *cluster* fsid under Rook's own
  convention. Both work; they're just two different parties (ceph-volume vs. Rook) independently
  using the same header field for their own purposes.
- **Splitting one disk into multiple raw-mode OSDs works as expected.** Both partitions discovered
  correctly via `raw list` alongside the cluster's existing OSD, with zero LVM tags anywhere,
  activated independently, and purged cleanly with no cross-interference.

### Verified: KMIP as the actual source of truth, not the mon

The tests above used locally-generated keys to isolate the raw-mode mechanics. This one closes the
loop by sourcing the passphrase from a real KMIP server instead, using the exact object shape Rook's
own client registers (`pkg/daemon/ceph/osd/kms/kmip.go`'s `registerKey()`/`getKey()`: a
`SymmetricKey` object, AES, `CryptographicUsageMaskExport`), over the same mutual-TLS client
certificate Rook already uses for a cluster's existing PVC-based OSD — not a standalone protocol test
against a different server.

1. **Register** a freshly-generated key to KMIP, simulating prepare time → got back a
   `UniqueIdentifier` (incrementing cleanly from the existing OSD's own identifier — same server,
   same keyspace, same cert).
2. Used that key, straight out of the Register response, to `ceph-volume raw prepare --dmcrypt` a
   fresh OSD. Succeeded identically to the locally-generated-key test above.
3. Closed the mapper, simulating a cold boot — bluestore label unreadable while locked, as before.
4. At simulated activate time, from a separate invocation holding **only the identifier, no access to
   the original key material**, did a fresh KMIP `Get` call → got back the identical key bytes.
5. Used that independently-fetched value to `luksOpen` the device and run `ceph-volume raw activate`
   → succeeded, bluestore label fully readable again.

This confirms KMIP can genuinely be the source of truth for the LUKS passphrase in raw mode, end to
end, with no dependency on the mon's lockbox/config-key store at any point — the mon was only ever
consulted for `osd new` (minting the OSD ID itself, unrelated to the encryption key). Everything
needed to decrypt the OSD came from KMIP.

### Recommended implementation

No ceph-volume change is required at all for the KMS piece — `raw.py` already honors
`CEPH_VOLUME_DMCRYPT_SECRET` at both prepare and activate time, as verified above. The work is
entirely inside Rook, and closely mirrors what `startProvisioningOverPVCs()` already does:

1. **Identifier.** PVC mode keys the KMS secret name off `pvc.ClaimName`
   (`GenerateOSDEncryptionSecretName`), known before the provisioning job starts. Host-based
   provisioning has no equivalent identifier that early — `osd_fsid` is normally generated by
   `ceph-volume` itself during `prepare()`, and `raw.py` already accepts a pre-set one via
   `args.osd_fsid`. Proposal: the node provisioning job pre-generates one UUID per device it intends
   to provision, uses it as the KMS secret name, and passes it explicitly via
   `ceph-volume raw prepare --osd-fsid <uuid>` instead of letting `ceph-volume` generate one. This
   mirrors Rook already owning OSD identity for the PVC path (see Verified section above) rather than
   depending on ceph-volume's native LUKS2-header discovery.
2. **Device provisioning.** For a single whole-device OSD, this is a direct `ceph-volume raw prepare
   --data <device>` call, no partitioning needed. For splitting one physical device into multiple
   OSDs (today's `osdsPerDevice`) or sharing one fast device as DB/WAL across several OSDs, Rook's own
   code must pre-partition the device and pass each resulting block device path explicitly — see
   Drawbacks; this is the main piece of net-new logic this proposal requires beyond KMS wiring itself.
3. **Key generation and KMS push.** The host-based provisioning path gains the same
   `kms.NewConfig()` / `GenerateDmCryptKey()` / `kmsConfig.PutSecret(identifier, key)` sequence
   `startProvisioningOverPVCs()` already runs, once per device/partition, before the provisioning job
   starts.
4. **Env var injection.** The provisioning job's pod spec gains `CEPH_VOLUME_DMCRYPT_SECRET` and the
   KMS config env vars (`kms.ConfigToEnvVar`), plus Vault TLS volume mounts where relevant, exactly
   as `provision_spec.go` already does under `onPVC()`.
5. **Activation.** The OSD deployment's init container fetches the key from KMS and must run
   `cryptsetup luksOpen` itself before `ceph-volume raw activate` runs — confirmed above that `raw
   activate` cannot unlock a cold device on its own; its discovery depends on the bluestore label,
   which only exists on the already-open mapper. This is exactly what
   `getPVCEncryptionOpenInitContainerActivate` in `spec.go` already does for PVC-mode OSDs, so the
   host-based init container is the same pattern applied to a non-PVC pod, not a new mechanism.

**Maintainability choice:** rather than a parallel set of KMS-wiring code for the new path, factor the
shared logic (`kms.NewConfig`/`PutSecret`/`ConfigToEnvVar`, the init container template) into helpers
parameterized on the identifier, called from both `startProvisioningOverPVCs()` and the new host-based
path. Both paths end up driving the same `raw.py` mechanism; there is no reason for Rook to carry two
independent implementations of "push a key to a KMS and get it back into a
`CEPH_VOLUME_DMCRYPT_SECRET` env var."

