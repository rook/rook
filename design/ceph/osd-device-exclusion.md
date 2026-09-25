# Design: OSD device exclusion

Related issues: [rook/rook#16077](https://github.com/rook/rook/issues/16077), [rook/rook#16535](https://github.com/rook/rook/issues/16535) (users reaching for the discovery daemon's udev blacklist and being surprised by its actual, discovery-only effect).

## Problem

A physical disk that should never be used again can keep re-entering the OSD provisioning pipeline. The motivating case is a failing SAS drive that repeatedly dies and revives: each revival enumerates under a new kernel name (`sdo` one day, `sdk` the next), so no name-based configuration can refer to it durably. After the admin purges its OSD and deletes the deployment, nothing prevents the disk's next revival from being re-adopted as a new OSD — or re-reported from its stale BlueStore label and rebuilt — and every cycle costs manual work.

Every device-selection mechanism in `CephCluster.spec.storage` is an allowlist: `useAllDevices`, `deviceFilter`, `devicePathFilter`, and the explicit `devices` list. The filters are RE2 regular expressions, which have no negative lookahead, so "every device except this one" is not practically expressible; switching a `useAllDevices` cluster to explicit per-node device lists abandons automated selection entirely and is itself keyed by unstable names. The one knob named "blacklist", `DISCOVER_DAEMON_UDEV_BLACKLIST`, only filters udev *events* in the optional discovery daemon; it never gates provisioning.

This design adds a declarative device exclusion list ("deny list") to the CephCluster storage spec, keyed by stable device identity, physical slot, or model — plus reporting when a live OSD occupies an excluded device, so the remaining manual removal is prompted rather than discovered. The exclusion entry changes what Rook will *provision*; it never changes Ceph state. Removing an OSD stays an explicit admin action through the existing tools (`kubectl rook-ceph rook purge-osd`, the osd-purge Job).

## Goals and non-goals

Goals:

- Permanently prevent OSD provisioning on specific physical devices, identified by stable properties (serial, WWN, Ceph device ID), physical slot path, or model — never by kernel name.
- Surface when an existing OSD sits on an excluded device, so the remaining manual step is prompted rather than discovered.

Non-goals:

- **Automated removal of excluded OSDs.** An exclusion entry triggers no Ceph state change — no `out`, no pod stop, no purge. Removal remains an explicit admin decision through the existing tools; automation of it is left to a separate design.
- **PVC-backed OSDs** (`storageClassDeviceSets`). Device choice there belongs to the PV provisioner; out of scope.
- **Discovery daemon inventory.** The discovery daemon's reports are not filtered by exclusions; they gate nothing. (The operator's hotplug *trigger* comparison is exclusion-aware — see Provisioning-time enforcement — but the reported inventory stays complete.)
- **Host-level device hiding** (udev rules, sysfs SCSI deletes). Imperative, reboot-fragile, and invisible to GitOps; exclusion belongs in the cluster spec.

## API

Exclusions are a single cluster-level list on `StorageScopeSpec`, with optional per-entry node scoping. They are deliberately NOT part of the `Selection` struct: node-level `Selection` content is resolved with node-overrides-cluster semantics and is discarded entirely under `useAllNodes: true` (the operator logs that `nodes` entries "will be IGNORED"), and an exclusion must survive both behaviors. A top-level list flows into the operator's resolved storage spec unchanged regardless of `useAllNodes`, and per-entry scoping expresses the one genuinely node-local case (slot exclusion — identical hardware yields identical `by-path` strings on every node) without a second per-node list to merge.

```go
type StorageScopeSpec struct {
	...
	// ExcludedDevices lists devices that must never be used for OSDs (data or
	// metadata), even if explicitly listed in devices. Exclusion applies on
	// every node unless an entry is scoped with nodes.
	// +kubebuilder:validation:MaxItems=256
	// +optional
	ExcludedDevices []ExcludedDevice `json:"excludedDevices,omitempty"`
}

// ExcludedDevice identifies a device (or class of devices) to exclude.
// Exactly one selector field must be set.
type ExcludedDevice struct {
	// Serial is the printed disk serial number, as shown on the drive label,
	// by smartctl, and as the trailing component of `ceph device ls` IDs
	// (exact match; see Matching semantics for the udev fields consulted).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Serial string `json:"serial,omitempty"`
	// WWN is the disk world wide name (exact match, case-insensitive, with or
	// without the 0x prefix). Provisioning-gate only: WWNs do not appear in
	// `ceph osd metadata`, so wwn entries cannot trigger reporting.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	WWN string `json:"wwn,omitempty"`
	// CephDeviceID is the device ID as printed by `ceph device ls`
	// (exact match).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	CephDeviceID string `json:"cephDeviceID,omitempty"`
	// DevicePathRegex is an RE2 regular expression matched against persistent
	// device paths. Use for slot/bay exclusion via /dev/disk/by-path.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +optional
	DevicePathRegex string `json:"devicePathRegex,omitempty"`
	// ModelRegex is an RE2 regular expression matched against the device
	// model string. Use to exclude an entire drive model.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +optional
	ModelRegex string `json:"modelRegex,omitempty"`
	// Nodes limits the entry to the named nodes (Kubernetes node names, as in
	// spec.storage.nodes). Empty means the entry applies on every node.
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Nodes []string `json:"nodes,omitempty"`
	// Comment is a free-form operational note (why/when the device was
	// excluded). It appears in logs, Events, and status when the entry
	// matches.
	// +kubebuilder:validation:MaxLength=256
	// +optional
	Comment string `json:"comment,omitempty"`
}
```

Example:

```yaml
spec:
  storage:
    useAllNodes: true
    useAllDevices: true
    excludedDevices:
      - serial: WSD4QCXX
        comment: "flapping SAS drive, RMA 2026-08-07"
      - cephDeviceID: SEAGATE_ST8000NM014A_WSD4PT3Q
      - modelRegex: "(^|_)ST3000DM001" # fleet-wide model exclusion; (^|_) tolerates vendor-prefixed IDs report-side
      - devicePathRegex: "-sas-.*-phy3-"
        nodes: ["node042"]           # bay ban, scoped to one node
        comment: "bay 3 backplane flaky"
```

Node scoping note: at provisioning time the effective node identity under `useAllNodes` is the hostname (`k8sutil.GetNodeHostNames`), and at reporting time it is the `hostname` field of `ceph osd metadata`, which equals the Kubernetes node name in rook clusters (Ceph rewrites it from `$NODE_NAME`, which rook sets on all daemon pods). The implementation normalizes once, in the operator: CR node names and `osd metadata` hostnames are both resolved through the Node objects to the same `kubernetes.io/hostname` identity used at provisioning.

Validation:

- A CEL rule on `ExcludedDevice` enforces that exactly one selector field is set (`nodes` and `comment` excluded from the count). With the list bounded by `MaxItems=256` and all-scalar fields bounded by `MaxLength`, the rule's static cost is far inside the apiserver's per-rule budget; the bounds above are part of the API, not implementation detail — an unbounded list of all-optional structs pushes any natural exactly-one expression over the estimator's limit and the CRD would be rejected at registration. A CRD-registration test asserts the manifests apply cleanly.
- `MinLength=1` on every selector closes the empty-string hole: without it, `serial: ""` satisfies exactly-one while matching nothing — a silently inert entry.
- CEL cannot verify that a regular expression compiles. The operator validates every `devicePathRegex` and `modelRegex` at reconcile time and **fails the reconcile** on a compile error. Fail-closed is deliberate: an exclusion that silently does not apply is the precise failure mode this feature exists to prevent.

## Matching semantics

Exclusion is evaluated at two points with different information sources: the provisioning gate reads udev properties of a present disk, and the reporting path reads what Ceph recorded about an OSD's device in `ceph osd metadata` (`device_ids`, `device_paths`, `hostname` — Ceph records no standalone serial, WWN, or model fields; both device fields are comma-joined `devname=value` strings, which the reporting matcher splits and strips of the `devname=` prefix before applying the table to each value). The contract below defines, per selector, exactly what each side matches; selectors that cannot be evaluated on the reporting side are called **reporting-blind** and are surfaced as such (see Reporting).

| Selector | Provisioning gate matches | Reporting matches | Notes |
|---|---|---|---|
| `serial` | set membership over udev `ID_SCSI_SERIAL`, `ID_SERIAL_SHORT`, and `ID_SERIAL` (exact) | the trailing `_<serial>` component of each `device_ids` value (suffix match) | `ID_SERIAL` alone is NOT the printed serial: it is the NAA identifier on SAS and a `MODEL_SERIAL` composite on SATA/NVMe. Rook currently collects only `ID_SERIAL`; the implementation adds `ID_SCSI_SERIAL` and `ID_SERIAL_SHORT` to `LocalDisk`. |
| `wwn` | udev `ID_WWN` / `ID_WWN_WITH_EXTENSION` (exact, case-insensitive, `0x` optional) | **reporting-blind** — no WWN exists anywhere in `osd metadata` | Gate-only. |
| `cephDeviceID` | the ID derived from udev fields following Ceph's `get_device_id` three-tier algorithm (exact) | `device_ids` values (exact) | Derivation is `ID_VENDOR_ID_MODEL_ID_SCSI_SERIAL` when all three exist, else `ID_MODEL_ID_SERIAL_SHORT` (no vendor — the common SATA case), else raw `ID_SERIAL`; spaces become underscores. The implementation reproduces this algorithm (kept in sync with ceph-volume's `_get_device_id`) using the same new udev fields as `serial`. |
| `devicePathRegex` | every persistent path symlink plus `/dev/NAME` — the same set `devicePathFilter` matches | `device_paths` values, which contain **only** `/dev/disk/by-path` links | A pattern targeting `by-id`/`by-uuid` links gates provisioning but is reporting-blind; slot bans written against `by-path` work on both sides. |
| `modelRegex` | the udev model string | best-effort against `device_ids` values, whose model component has spaces replaced by underscores | Patterns containing spaces should use `[ _]` to match on both sides; identity selectors give the most reliable reporting. |

Rules:

1. **Exclusion is absolute.** It is evaluated before and above every allow mechanism: `useAllDevices`, `deviceFilter`, `devicePathFilter`, explicit `devices` entries, and `metadataDevice` references. An explicit allow that matches an exclusion is suppressed with a warning (see below), never honored — a stale `devices: [{name: sdk}]` entry must not resurrect an excluded disk that inherited the kernel name.
2. **Node scoping.** An entry with `nodes` set applies only on those nodes; all other entries apply everywhere. There is no node-level exclusion list to merge or override — the cluster-level list is the single source of truth and is unaffected by `useAllNodes`.
3. **Identity matching is best-effort by nature.** A device whose udev identity cannot be read cannot match an identity selector (fail-open). A `devicePathRegex` exclusion on its bay still catches it. This limitation is documented rather than papered over with a knob. Note the common transports are already imperfect without this: the printed serial lives in different udev fields per transport, which is exactly why `serial` matches a set of fields rather than one.

## Provisioning-time enforcement

The operator resolves the effective exclusion list per node (cluster list filtered by each entry's `nodes` scope) and passes it to the OSD prepare job as JSON in a new `ROOK_EXCLUDED_DEVICES` environment variable, alongside the existing `ROOK_DATA_DEVICES`/`ROOK_DATA_DEVICE_FILTER` variables (the provision command binds env vars onto its flags via `SetFlagsFromEnv`, so this surfaces as an `--excluded-devices` flag on `rook ceph osd provision`). There is no operator↔prepare version-skew concern: the prepare container runs the cluster's Ceph image with the rook binary injected from the operator image, so producer and consumer are always the same rook version.

In the prepare job's `getAvailableDevices` loop, every discovered device is checked against the list **before** desired-device matching:

- On match, the device is skipped with a log line naming the matched selector and comment, e.g. `skipping device "sdk": excluded by cluster spec (serial=WSD4QCXX, "flapping SAS drive, RMA 2026-08-07")`.
- If the excluded device was also explicitly requested (a `devices` entry by name or path), the conflict is recorded in the prepare job's orchestration status, and the operator emits a Kubernetes warning Event (`ExcludedDeviceConflict`) on the CephCluster naming both the exclusion entry and the explicit request.
- If a configured `metadataDevice` matches an exclusion, provisioning for the affected OSDs **fails loudly** with an explicit error rather than proceeding without a metadata device.

The gate also covers the **existing-OSD enumeration**. The prepare job's `ceph-volume lvm list`/`ceph-volume raw list` pass — which re-reports OSDs from BlueStore labels so the operator can rebuild their deployments — is filtered against the exclusion list by underlying device identity, at the same points the destroyed-OSD filter applies today. This matters after a manual purge: purge removes the OSD id from the CRUSH tree entirely, so the destroyed-id filter cannot recognize a purged-but-unzapped disk, and its next revival would otherwise be re-reported to the operator and rebuilt as a crashlooping, auth-less deployment on the banned device — or squat on a since-recycled OSD id and block the legitimate OSD's deployment.

Enumeration suppression is conditioned on **cluster membership**, following the destroyed-id filter's own pattern of conditioning on cluster state rather than raw device identity: an enumerated OSD is suppressed only when the cluster no longer knows it — its id is absent from the osdmap, or the id exists with a different OSD uuid (the id was recycled to another disk). A live member — id present with matching uuid — is never suppressed, however its device matches the exclusion list: rebuilding an existing, authed OSD's deployment is not provisioning, and refusing it would turn a report-only entry into an availability action (in disaster recovery, exactly when the cluster is most fragile). The violation report is the surfacing mechanism for live members; the gate is the wall against dead ones. A suppressed stale OSD is recorded in the orchestration status and surfaced as a warning Event, never silently dropped. The exclusion entry is the durable suppression: keep it until the disk is pulled or zapped.

Two OSD-replacement paths get the same treatment. The replacement's metadata recovery re-provisions with a surviving DB/WAL LV chosen from the host, deliberately reading no spec — so the configured-`metadataDevice` rule cannot catch it; the recovered LV's underlying device (VG→PV parents resolved to udev identity) is therefore checked against the exclusion list, and a match **fails that replacement slot loudly**, mirroring the excluded-`metadataDevice` rule. And when a destroyed replacement slot is waiting for a blank device while candidate devices on that node were exclusion-suppressed in the same prepare run, the suppression is recorded in the orchestration status and the operator emits a warning Event (`ExcludedDeviceBlocksReplacement`) — a banned incoming disk must not silently wedge a replacement at ready-for-swap.

When the discovery daemon is enabled, its per-node inventory ConfigMaps additionally *trigger* OSD orchestration on device-list changes (the hotplug watcher). That trigger comparison is exclusion-aware: devices matching an exclusion entry are dropped from both list snapshots before comparing, so an unzapped excluded flapper's revivals schedule nothing — ending the fleet-wide no-op prepare waves of the #16535 churn class — while any change involving a non-excluded device still triggers. Deleting an entry needs no hotplug event to take effect: the spec edit itself triggers a CephCluster reconcile. The decision is evaluated **per target cluster at enqueue time**: the watch's update handler owns the delta computation and the fan-out, suppressing the enqueue only for clusters whose own spec filters the entire delta — one cluster's exclusions must never eat a trigger another cluster needed. Node scoping resolves the ConfigMap's node label through the same normalizer as every other matcher call-site; if Node resolution fails, scoped entries are treated as non-matching for this decision. The daemon's reported inventory itself stays complete (the non-goal above stands): only the decision to schedule work consults exclusions. The filter needs the same identity fields as the prepare-side matcher, so the udev collection additions apply to the discovery probe as well; and if the spec's patterns cannot be compiled, the watcher falls back to the unfiltered comparison — failing open toward triggering is the safe direction: the triggered reconcile itself fails closed on the invalid pattern, and no prepare launches until the spec is fixed.

## Reporting

OSDs already deployed on excluded devices are detected by the OSD health monitor on its existing interval: `ceph osd metadata` (mon-store persisted; available for down OSDs until purge) is matched against the exclusion list using the reporting column of the matching contract.

Violations surface in a **dedicated status section owned exclusively by the OSD health monitor** — deliberately not inside `status.storage`, whose existing writer rebuilds that struct from zero on every OSD orchestration and would wipe any co-located rows. The monitor maintains its section with read-modify-write against the previously published status plus a full-set memory: rows are ordered deterministically (ascending OSD id), `since` values survive rewrites, and `ExcludedDeviceInUse` Events are deduplicated against the complete computed violation set — held by the monitor and re-seeded from published status on operator start (worst case, one repeated Event per restart) — never against the truncated rows alone. Violations beyond the row cap are represented by `violationCount` and one aggregate warning Event; per-row Events never fire for beyond-cap rows:

```go
// CephClusterStatus gains:
ExcludedDevices *ExcludedDevicesStatus `json:"excludedDevices,omitempty"`

type ExcludedDevicesStatus struct {
	// ViolationCount is the total number of live OSDs on excluded devices.
	ViolationCount int `json:"violationCount"`
	// Violations lists up to 32 violations; ViolationCount carries the rest.
	Violations []ExcludedDeviceViolation `json:"violations,omitempty"`
	// ReportingBlindEntries names exclusion entries that can gate
	// provisioning but can never match `ceph osd metadata` (e.g. wwn
	// selectors), and therefore cannot trigger reporting.
	ReportingBlindEntries []string `json:"reportingBlindEntries,omitempty"`
}

type ExcludedDeviceViolation struct {
	OSD             int         `json:"osd"`
	Host            string      `json:"host"`
	DeviceID        string      `json:"deviceID,omitempty"`
	MatchedSelector string      `json:"matchedSelector"` // e.g. "serial=WSD4QCXX"
	Message         string      `json:"message,omitempty"`
	Since           metav1.Time `json:"since,omitempty"`
}
```

`Message` states what remains for the admin: for an `up`+`in` OSD it names the manual removal step; for an OSD already `out` it notes that the OSD is eligible for manual purge. A violation disappears once the OSD is purged (purge removes its `osd metadata`). A warning Event (`ExcludedDeviceInUse`) fires when a violation is first observed; entries using reporting-blind selectors raise a warning Event (`ExcludedDeviceUnreportable`) once, since an OSD on such a device would never be reported.

## Removing an excluded OSD (manual)

Removal is out of scope for this design by decision, not omission: an exclusion entry never changes Ceph state, and every removal step stays an explicit admin action through the existing, purpose-built tools. The composition is:

1. The violation report (status row + Event) surfaces the OSD sitting on an excluded device.
2. The admin removes it with the standard runbook — `ceph osd out`, wait for rebalance, then `kubectl rook-ceph rook purge-osd <id>` or the osd-purge Job — exactly as documented today.
3. Purge erases the OSD's `osd metadata`, clearing the violation; the provisioning gate and the enumeration filter then keep the disk's later revivals out permanently, with no further admin attention.

The entry is what makes the manual purge *stick* — today the purged disk's next revival is silently re-adopted; with the entry it is refused and the refusal is logged and reported.

## Interaction with OSD replacement

The OSD replacement flow (`design/ceph/osd-replacement.md`) and device exclusion are complementary: replacement answers "swap this disk in this bay, preserving the OSD id" — per-OSD, admin-confirmed, a slot-preserving `destroy` with a ready-for-swap signal — while exclusion answers "never use this physical device (or model, or bay) again" — spec-level, one entry may cover many OSDs, and it follows the disk wherever it is inserted.

When both intents apply to one disk — ban the physical device forever AND pass its slot and OSD id to a successor — the procedure, with the exclusion entry hardening each step:

1. **Exclude the failing disk by identity.** Add an entry for its serial (from `ceph device ls` or the drive label). From this moment no provisioning path can place an OSD on that physical disk — not new-device selection, not the existing-OSD enumeration, and not the re-provisioning step that completes a replacement — even if the disk revives mid-procedure.

   ```yaml
   spec:
     storage:
       excludedDevices:
         - serial: WSD4QCXX
           comment: "failing drive, node042 bay 3; RMA 12345, replaced 2026-08-10"
   ```

2. **Mark the OSD for replacement.**

   ```console
   kubectl -n rook-ceph annotate deployment rook-ceph-osd-273 \
     osd.rook.io/replace="yes-really-replace-osd-273"
   ```

   No purge is involved: the replacement flow marks the OSD out, drains it fully (`safe-to-destroy`), destroys it slot-preservingly (the id stays in the CRUSH tree), and announces ready-for-swap.

3. **Swap the disk.** The prepare job re-provisions the preserved OSD id onto the replacement disk, whose serial passes the gate. This is the step the exclusion entry hardens. A revived failing disk with its BlueStore label intact is already ineligible — the replacement flow stakes its swap detection on that signature, and unreadable disks fail closed. The entry closes the label-loss variants: an admin who zaps the old disk in place before pulling it (a data-wipe habit carried over from purge-based runbooks — without the entry, the zapped failing disk becomes exactly the blank device the swap detection is waiting for and is immediately re-provisioned into the destroyed slot) and failing media that presents blank without erroring. With the entry, only the new disk can be chosen regardless of the old disk's label state.

4. **The pulled disk stays banned.** If it is later shelved into this or any other node, the gate refuses it, and a violation is reported should an OSD ever be found on it.

For entries created as part of a swap procedure, prefer identity selectors (`serial`, `wwn`, `cephDeviceID`). A `modelRegex` or bay-scoped `devicePathRegex` that matches the **incoming** replacement disk refuses it — correctly, but the replacement then holds at ready-for-swap until the entry is adjusted or a different disk is inserted; the `ExcludedDeviceBlocksReplacement` Event surfaces this state. RMA programs routinely substitute the same model, which a fleet-wide model ban would refuse.

## Other interactions and edge cases

- **Downgrade / version skew.** An older operator ignores `excludedDevices` entirely and could re-adopt an excluded device; an older CRD prunes the field on apply. Called out in the documentation and release notes.
- **`removeOSDsIfOutAndSafeToRemove`.** Unchanged by exclusion. The existing flag deletes only the Kubernetes deployment of a down+out OSD once `safe-to-destroy` passes — it never purges — and a live excluded member's deployment deleted this way is re-reported and rebuilt exactly as today (the membership-conditioned gate does not suppress live members). Once the admin purges the OSD, the id leaves the cluster and the gate suppresses the disk's revivals permanently.
- **Multipath.** `serial`/`cephDeviceID` catch every path to the disk; `devicePathRegex` is per-path by nature.
- **Encrypted OSDs.** Gate-side matching happens on the raw device before dmcrypt layering.
- **`cleanupPolicy` / zapping.** Unaffected. Zapping an excluded device is fine; it simply stays unused.
- **Metadata devices.** Exclusion applies to metadata (db/wal) placement as well as data devices.
- **External clusters.** The OSD health monitor does not run for external clusters, so reporting is inert there; the provisioning gate is moot (no operator-managed OSDs).

## Testing

- Table-driven unit tests for the matcher, per adapter: every selector type against both information sources, the per-transport serial field set (`ID_SCSI_SERIAL`/`ID_SERIAL_SHORT`/`ID_SERIAL`), the three-tier `cephDeviceID` derivation, `device_ids` suffix matching, WWN normalization, node scoping with hostname normalization, devices with missing identity fields, and reporting-blind classification.
- `getAvailableDevices` unit tests extended with excluded-device scenarios, including the explicit-`devices` conflict path and the excluded-`metadataDevice` failure.
- Existing-OSD enumeration gate: a stale BlueStore-labeled excluded disk is suppressed from the orchestration status (with its warning Event) instead of re-reported; a purged id never re-enters the deployment set; a LIVE member (id present with matching uuid) on an excluded device is re-reported and its deployment rebuilt despite the entry; an id recycled to a different uuid is suppressed.
- Hotplug trigger filter: an excluded flapper's revival schedules no orchestration; a change including any non-excluded device still triggers; per-cluster evaluation (one cluster's exclusions never suppress another cluster's enqueue); a pattern-compile failure falls back to the unfiltered comparison.
- Replacement composition: the recovered-DB-LV gate fails a slot on an excluded metadata device; `ExcludedDeviceBlocksReplacement` fires when a waiting destroyed slot coincides with suppressed candidate devices; the zap-in-place scenario (blanked old disk) is refused by the gate.
- Health-monitor unit tests against canned `ceph osd metadata` via the fake executor: violation detection per selector, `message` content for in and out OSDs, and violation clearance on purge.
- Status tests: read-modify-write preserves `since` across health cycles and across OSD orchestrations; deterministic row ordering; the 32-row cap, `violationCount`, and the beyond-cap aggregate Event; full-set Event dedup re-seeded after an operator restart; `reportingBlindEntries`.
- CRD validation: CEL exactly-one rules (including rejection of multi-selector and empty-string entries), and a registration test applying the generated CRDs to a live apiserver so a CEL cost-budget regression fails in CI by construction.
- Integration (follow-up): a canary-style test that excludes one loop device by path and asserts its twin is provisioned while it is skipped.

## Documentation

- New subsection in the cluster CRD storage-selection documentation: the example above, the per-selector capability table (including reporting-blind selectors), and the manual-removal runbook for reported violations.
- The replacement section of the OSD management documentation (`Documentation/Storage-Configuration/Advanced/ceph-osd-mgmt.md`) gains the combined exclude-and-replace procedure of the interaction section above.
- Commented-out example in `deploy/examples/cluster.yaml`.
- `PendingReleaseNotes.md` entry (implementation PRs, not this design PR).

## Alternatives considered

- **Negated `deviceFilter`/`devicePathFilter`.** RE2 has no negative lookahead; complement-of-a-literal is not practically expressible, and filters are allowlists by contract.
- **Explicit per-node `devices` lists.** Abandons automated selection on every node to solve a per-device problem, and is itself keyed by unstable kernel names unless every entry uses full `by-id` paths — at which point each node's list must be hand-maintained anyway.
- **`DISCOVER_DAEMON_UDEV_BLACKLIST`.** Wrong layer: filters udev event processing in the optional discovery daemon; gates nothing in provisioning.
- **Host-side hiding (udev rules, `echo 1 > /sys/block/X/device/delete`).** Works until reboot or re-enumeration, requires per-host imperative management outside the cluster spec, and is invisible to anyone reading the CephCluster.
- **A separate CRD for exclusions.** A new controller, RBAC, and cross-resource watches for no added capability; the storage spec is where every other device-selection decision already lives.
- **Operator-level configuration (env var / ConfigMap).** Not per-cluster, weakly validated, and invisible in the cluster spec.
- **Exclusions under `spec.storage.nodes[].excludedDevices` (the `Selection` struct, per-node lists with union merge).** Rejected: the operator discards `nodes[]` entirely under `useAllNodes: true` before node resolution ever runs (with only an operator-log warning), and resolves node-level `Selection` content with override semantics otherwise — so per-node exclusion lists would be silently inert in the dominant hands-off configuration, or would require a second node-config channel with bespoke union-merge rules; nesting the entry validation inside unbounded `nodes[]` also breaks the CEL cost budget discussed under Validation. Per-entry `nodes` scoping on one cluster-level list expresses the same intent without any of these failure modes.
