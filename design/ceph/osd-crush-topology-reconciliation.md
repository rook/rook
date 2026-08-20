---
title: osd-crush-topology-reconciliation
target-version: release-1.21
---

# OSD CRUSH Topology Normalization

## Summary

Rook reads a node's topology labels once per OSD, in the OSD prepare job, and bakes the result
into the OSD Deployment as a `ceph-osd --crush-location=` argument. Changing a topology label on a
node that already hosts OSDs has no online effect on the CRUSH map, and the CRUSH hierarchy above
the host level drifts permanently away from what the labels describe.

This proposal adds an opt-in, continuously reconciled mode in which **node labels are the only
source of truth for the CRUSH bucket hierarchy beneath the configured root**. While enabled, Rook
computes the tree the labels describe, diffs it against the live map, and converges the map to it
in a single atomic operation — creating, moving, and **removing** buckets as required. Structure
the labels do not describe is not preserved; it is what normalization removes.

### Why the OSD's own crush-location cannot deliver this

Re-rendering `--crush-location` and restarting the OSD does not work. Verified against Ceph
v19.2.2 source:

- A `ceph-osd` issues `osd crush create-or-move` at startup (`src/osd/OSD.cc`, gated on
  `osd_crush_update_on_start`, default true).
- `CrushWrapper::check_item_loc()` iterates the CRUSH type table in ascending type-id order, skips
  types absent from the supplied location, and returns at the first type present. Rook's location
  contains only `root=`, `host=`, and topology levels, so the lowest type present is always
  `host`: an OSD already in its correct host bucket is reported as already placed, and nothing
  above `host` is examined — even when the host bucket sits under the wrong parent.
- Were that to fail, `insert_item()` creates missing ancestors while climbing but breaks at the
  first bucket that already exists, so for an existing host it too stops at `host`.

So for OSDs on existing hosts, no per-OSD mechanism can restructure or remove anything above
`host`. Normalization is a map-level operation. (For a *new* host the daemon does create
ancestry, which several parts of this design have to account for.)

### The supported workaround, and why it is not enough

`Documentation/CRDs/Cluster/ceph-cluster-crd.md` documents the limitation and a remedy: applying
labels to an existing cluster "requires removing each node from the cluster first and then
re-adding it with new configuration to take effect. Do this node by node to keep your data safe!"
That works: the purge path runs `ceph osd crush rm <hostName>` unconditionally after each OSD
purge, swallowing the error while other OSDs remain, so the host bucket is removed by Ceph's own
emptiness check once the last OSD is gone, and re-provisioning rebuilds the labelled ancestry.
It costs a full backfill out and back per node, and it removes no stale intermediate buckets.

### Relationship to existing issues

Issue #17652's fix made a node topology label change wake the CephCluster reconcile. Nothing acts
on it. This proposal supplies the missing half, and the continuous model is what makes the wake-up
meaningful: a label edit is a declaration of desired state, and the reconcile converges to it.

## Goals

- While the mode is enabled, the CRUSH bucket hierarchy beneath the configured root is a pure
  function of node labels: buckets the labels describe exist and are correctly parented; buckets
  they do not describe are removed.
- The map transition is atomic — one mon operation, no intermediate hierarchy ever visible to
  CRUSH, and one planned rebalance.
- Anything that prevents a safe, complete normalization refuses the whole operation and reports
  precisely why, without disturbing any other part of cluster reconciliation. A refusal also
  freezes this feature's own map and Deployment writes. It does not freeze OSD provisioning: a
  genuinely new host's daemon still builds ancestry via `insert_item()`, so the map may gain
  buckets during a refusal — though never lose or re-parent existing ones — and the next diff
  reports them.
- The full desired-vs-actual diff — including the per-Deployment location and label changes — is
  visible before anything is modified, and durable.
- Default to today's behavior; nothing changes until the mode is explicitly enabled.
- Nothing in this design serializes reconciliation across CephClusters: multiple clusters managed
  by one operator reconcile in parallel, and every exclusion below is scoped to a single cluster.

## Non-Goals

- **Merging with hand-built structure.** There is no mode in which Rook preserves buckets the
  labels do not describe while managing the rest (see Alternatives). An administrator who wants
  hand-built structure keeps the feature off.
- **Removing OSDs or hosts.** Normalization restructures the bucket hierarchy above the host
  level. OSD and host lifecycle remain the provisioning and purge paths' job. (Rook's only
  existing bucket removals — the purge path's `osd crush rm <host>` and the crushRoot switch's
  `osd crush rm default` — are single-bucket and guarded; fleet-scale removal is what is new
  here.)
- **Device classes.** Rook sets device classes per OSD (`osd crush set-device-class`); Ceph
  derives the `<bucket>~<class>` shadow hierarchies itself and Rook never creates them. Shadow
  buckets never appear in the decompiled text at all (`~` is not a valid CRUSH name character;
  the compiler rebuilds shadow roots from per-bucket class-id declarations), so the diff cannot
  touch them. Rule *steps* may reference `<bucket>~<class>` names and class-scoped placement
  happens through shadow subtrees; refusals 4 and 10 below account for both.
- **CRUSH rules and tunables.** Normalization edits the bucket and type sections of the map only.
  Rules are pool reconciliation's job; a rule that references a bucket normalization would remove
  or rename is a refusal, not an edit.
- **Multiple CRUSH roots.** The tree beneath `spec.storage.config.crushRoot` is normalized; other
  roots' bucket structure is untouched and invisible to the diff. (The type table is global, so
  one narrow interaction exists — refusal 2 below.)

## Design

### The model

```text
desired tree = f(crushRoot, declared label list, nodes with Rook OSDs)
```

- The root comes from `spec.storage.config.crushRoot` (default `default`), as today. Labels
  describe everything beneath it.
- The declared label list (below) defines which CRUSH levels exist and their order, highest
  first. For each node hosting Rook OSDs, the node's values for the declared labels define the
  chain of buckets from the root down to its host bucket.
- Host bucket names derive from the node as today — the hostname label under `useAllNodes`, the
  node name the user wrote under an explicit `storage.nodes` list, dot→dash normalized; the PVC
  name for portable PVC OSDs (see Refusals). Host names are *not* products of the declared-list
  rendering, which matters for identity matching below.
- Any bucket beneath the root that is not part of that computed tree is removed by the
  normalization.

Because desired state is a pure function of labels, there is no anchor, no per-host placement
walk, and no merge logic. A design that instead merges label-derived structure into a preserved
hand-built tree must answer where each host's managed levels end and the administrator's begin —
and that question has no stable answer (see Alternatives). This model never asks it.

**Bucket identity.** The desired tree is matched to the actual tree bottom-up, keyed by bucket
**id**: host buckets match by name (node-derived, stable), and each level above matches by
(type, child-**id** set). Keyed by ids the match is stable under any change to how declared-level
names are rendered; keyed by names it would not be. A matched bucket keeps its CRUSH id, alg, and
hash; a matched bucket whose name differs (a level's value was renamed) is renamed in place in
the text, preserving its id, so fixing a typo in a rack name moves no data — unless a rule takes
the old name, which is refusal 4.

Item **weights** are preserved to within one unit of 1/65536, not bit-for-bit: bit-for-bit is
impossible through `crushtool --decompile`/`--compile`, whose weights print as `%.5f` of a
float32 quotient and re-parse as a truncated float32 multiply with no rounding anywhere in the
path (`src/crush/CrushCompiler.cc`). Measured over 1..2^20 fixed-point units (0–16 TB), 46.8% of
values lose exactly one unit and none gain; above 2^24 the error is two-sided, up to ±32 units.
The perturbation is bounded and self-limiting — a second round trip reaches a fixed point, total
drift ≤ 2 units in the ordinary range — and weights never enter the diff, so it does not
accumulate under continuous reconcile: a pass with no structural change performs no write. (The
existing rule-injection path has always had this property; this design is where it gets stated.)

The desired-tree rendering must be **byte-stable across Rook releases**: an armed cluster must
not see a non-empty diff — and therefore a rebalance — merely because the operator image was
upgraded. Two mechanisms enforce it (see Testing): a golden fixture pinning rendered output, and
a rendering-algorithm version persisted with the last-applied list, whose mismatch is a refusal
requiring a fresh dry-run review. The fixture is what keeps the version honest — a renderer edit
that skips the version bump would otherwise defeat the guard silently.

### The type table is Rook's to manage — and why

The declared list may contain **any** label keys, not only Rook's built-in topology labels. Each
key's suffix — the segment after the last `/`, or the whole key without one — names its CRUSH
type (the rule `kubernetesTopologyLabelToCRUSHLabel` already implements for the built-in set,
and which `spec.mon.failureDomainLabel` approximates with a first-`/` split). Types the map lacks
must be added, and that is not the end of it:

**CRUSH type ids are not ordering-free.** At the rule-execution layer they are pure names
(`mapper.c` compares exact ids), and `crushtool --compile` accepts any id assignment. But the
mon- and daemon-side location walks — `check_item_loc()`, `insert_item()` — iterate the type
table in **ascending id order** and treat that order as the hierarchy, and `OSDMap::check_health`'s
subtree-down aggregation walks ancestors on the same assumption. A custom type appended at the
next free id (12, above `root`'s 11) therefore breaks new-host ancestry construction and subtree
health reporting, even though placement itself would work.

The normalization therefore **owns the type table**, rewriting it so ascending id order matches
the declared hierarchy:

- `osd` keeps 0 and `host` keeps 1.
- The declared levels take ids 2..N+1, lowest declared level first.
- `root` takes the highest id.
- A built-in type absent from the declared list is dropped from the table once no bucket of that
  type remains anywhere in the map — beneath the managed root, normalization removes such
  buckets anyway. A bucket of an undeclared type under a **different** root is refusal 2:
  renumbering cannot guarantee ascending-id parentage for hierarchies the diff does not read.

Renumbering is safe **inside the CRUSH map** because the decompiled text binds types by name
there — rule steps say `type rack`, bucket declarations open with the type name — so recompiling
rebinds every in-map reference consistently, and bucket ids (which is what `choose_args` and
placement key on) are untouched. It is *not* safe for type ids persisted **outside** the map:
`pg_pool_t::peering_crush_bucket_barrier` and `OSDMap::stretch_mode_bucket` store raw type ids in
the OSDMap, nothing revalidates them when the table changes, and a stale barrier collapses
peering candidate selection (`get_parent_of_type` returns `0` — which is `osd.0`, not a sentinel)
rather than erroring. Refusal 9 exists precisely to keep such state out of scope.

Ceph's default table (verified, `src/osd/OSDMap.cc` v19.2.2): `osd` 0, `host` 1, `chassis` 2,
`rack` 3, `row` 4, `pdu` 5, `pod` 6, `room` 7, `datacenter` 8, `zone` 9, `region` 10, `root` 11.
External tooling that cached raw type ids observes the renumbering; that is disclosed as a
consequence, not hidden.

### API

```go
type StorageScopeSpec struct {
	// ...

	// CrushTopology, when set, makes node topology labels the only source of truth for the
	// CRUSH bucket hierarchy beneath the configured crushRoot. While confirmed, Rook
	// continuously reconciles the hierarchy to match the declared labels: buckets are
	// created, moved, and REMOVED so that the map matches what the labels describe.
	// Manual changes to the hierarchy are reverted on the next reconcile.
	// While confirmation is empty, Rook computes and reports the full diff — including
	// the per-OSD location changes — and modifies neither the CRUSH map nor any Deployment.
	// +optional
	CrushTopology *CrushTopologySpec `json:"crushTopology,omitzero"` //nolint:kubeapilinter // MinProperties cannot be applied to a struct pointer field
}

type CrushTopologySpec struct {
	// Confirmation must be set to "yes-really-normalize-crush-topology" for Rook to modify
	// the CRUSH map or any OSD Deployment — with one exception: a Deployment re-render
	// wave already in flight when this is cleared runs to completion against the
	// last-applied list, so the fleet is never left split between two renderings.
	// Normalizing an existing hierarchy moves data, restarts OSDs whose recorded location
	// changes, and removes buckets the declared labels do not describe. Once a
	// normalization has been applied, clearing this field stops further map changes but
	// does not revert anything, and OSD locations continue to render from the
	// last-applied label list.
	// +kubebuilder:validation:Pattern=`^$|^yes-really-normalize-crush-topology$`
	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Confirmation string `json:"confirmation,omitempty"`

	// CrushLocationLabels declares the node labels that define the CRUSH hierarchy, as
	// full Kubernetes label keys, ordered from the highest level to the lowest. Any label
	// key may be used; the segment after the last "/" names the CRUSH type for that level,
	// and Rook manages the map's type table to match. This list is the definition of the
	// tree: a level not declared here does not exist in the normalized hierarchy, and its
	// buckets are removed. The host level is always the lowest and is not declared.
	// Required.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=317
	// +listType=atomic
	CrushLocationLabels []string `json:"crushLocationLabels"`
}
```

| State | Spec | What Rook does |
|---|---|---|
| **Off** | `crushTopology` absent | Nothing computed. On a cluster that previously applied a normalization, OSD locations still render from the last-applied list (see Deployments below); on any other cluster, today's behavior exactly. |
| **Dry-run** | `crushTopology` set with `crushLocationLabels`; `confirmation` empty (`""` or omitted) | Computes the full diff every reconcile — bucket creates, moves, removals, per-OSD location changes, and refusals — and publishes it. **Writes nothing to Ceph or to any Deployment**; the diff ConfigMap, status summary, and refusal Events are published as usual. |
| **Armed** | `confirmation: yes-really-normalize-crush-topology`, `crushLocationLabels` set | Continuous normalization: the staged transition runs, and from then on the hierarchy is held to the labels. Every subsequent label edit self-applies one reconcile after it settles — the confirmation is a standing grant, not a per-plan approval. |

There is no valid empty-object state: `crushLocationLabels` is required with `MinItems=1`, so the
dry-run state is "list declared, confirmation empty" — the list must exist for there to be a diff
to compute. Clearing the confirmation returns to dry-run: future writes stop (an in-flight
Deployment wave excepted, per the godoc), nothing is reverted, and OSD locations continue to
render from the last-applied list (see Continuous semantics). The `Pattern` marker rejects any
value other than empty or the exact phrase at apply time.

**`crushLocationLabels` is required, with no default.** The obvious default — Rook's full
topology label set, as `csi.readAffinity.crushLocationLabels` defaults today — would be dangerous
here: that set includes `topology.kubernetes.io/zone` and `region`, which cloud controller
managers apply to nodes without administrator action, so a defaulted set would let
platform-applied labels define — and their absence remove — CRUSH structure nobody consciously
declared. Declaring the list is declaring the hierarchy; that act must be explicit.

**Why a confirmation string** rather than a boolean, matching `Migration.Confirmation`,
`OSDStore.UpdateStore`, and the `yes-really-destroy-data` family: while armed, edits to Node
labels — whose RBAC is typically much broader than CephCluster edit rights — drive CRUSH
restructuring and bucket removal. Arming is a standing grant, and the string's godoc says so
plainly. Per-plan confirmation is deliberately not offered: it is incoherent for a continuous
reconciler, and the dry-run state exists precisely so the first transition can be reviewed before
the grant is made.

**Why a pointer**: absent, dry-run, and armed must be three distinguishable states, and the
dry-run state carries content (the list), so the natural encoding is a pointer that is nil when
absent. `ObjectStoreSecuritySpec.SslOptions` and `ObjectStoreAccountSpec.RootUser` are the
post-linter pointer-struct precedents; both carry `//nolint:kubeapilinter` for the
MinProperties-on-pointer limitation, as this field does. `make go.kube-api-lint` (run with
`--new` in CI) drives the marker set above; the `yes-really-*` precedents carry `Pattern` only
because they predate the linter.

### Example

A cluster with a three-level hierarchy beneath the root — cloud zone, a custom machine-hall
level, and rack. Note `example.com/hall` is not one of Rook's built-in topology labels; its
suffix names the CRUSH type `hall`, which normalization adds to the map's type table.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  storage:
    config:
      crushRoot: default # optional; "default" if unset
    crushTopology:
      # confirmation is empty: dry-run. The full diff — bucket creates, moves,
      # removals, and per-OSD location changes — is published, and neither the
      # CRUSH map nor any Deployment is modified.
      crushLocationLabels:
        - topology.kubernetes.io/zone # highest declared level
        - example.com/hall            # custom key -> CRUSH type "hall"
        - topology.rook.io/rack       # lowest declared level; host is implicit
```

Nodes are labelled with a value for every declared key (`--overwrite` because the cloud
controller manager may already have set the zone label):

```console
kubectl label node node-a --overwrite topology.kubernetes.io/zone=z1 example.com/hall=hall2 topology.rook.io/rack=cab104
```

After reviewing the published diff, the administrator arms the mode:

```yaml
    crushTopology:
      confirmation: yes-really-normalize-crush-topology
      crushLocationLabels:
        - topology.kubernetes.io/zone
        - example.com/hall
        - topology.rook.io/rack
```

The normalized hierarchy for `node-a`, sketched from `ceph osd tree` in the toolbox (the ID
column shows *bucket* ids; the rewritten *type* table — here `osd` 0, `host` 1, `rack` 2,
`hall` 3, `zone` 4, `root` 5 — is visible via `ceph osd crush dump`):

```console
ID  CLASS  WEIGHT  TYPE NAME                STATUS  REWEIGHT  PRI-AFF
-1          ...    root default
-5          ...        zone z1
-9          ...            hall hall2
-12         ...                rack cab104
-2          ...                    host node-a
 0   ssd    ...                        osd.0      up   1.00000  1.00000
```

Any other bucket beneath `root default` — a hand-built `row` tier, a stale rack from an old
layout — is removed by the normalization. A node hosting OSDs that lacks any of the three
declared labels is refusal 1: the whole normalization is refused and the node is named in the
report. Relabelling `node-a` to `topology.rook.io/rack=cab105` later moves its host bucket into
`cab105` on the next applied pass — the label edit is the API.

### The reconcile, each pass

While the mode is set:

1. **Read once**, under the cluster's CRUSH lock (below): the compiled map via
   `GetCompiledCrushMap` (`osd getcrushmap`) for the text pipeline, the JSON model via
   `GetCrushMap` (`osd crush dump`) for the diff and type table, `crush_version` from
   `osd dump` (one new field on the existing `OSDDump` struct — `osd crush dump` does not carry
   it, and the `getcrushmap` status reply Rook currently discards lands on stderr), pool facts
   via `osd pool ls detail`, PG state counters, and the node list.
2. **Compute** the desired tree, the type-table assignment, and the diff: buckets to create,
   rename, re-parent, and remove, plus the per-OSD location and label changes that follow, and
   compile the desired text locally (`crushtool --compile` writes nothing to the cluster) for the
   placement check below.
3. **Check refusals** (below). Any refusal ends the step with no error surfaced to the
   surrounding reconcile — mons, mgr, RGW, CSI, and OSD provisioning proceed untouched — and
   freezes this feature's own map and Deployment writes. Refusals split by what they can
   publish: a refusal that invalidates the declared list itself (classes 2 and 5) publishes the
   reasons and the conflicting values — no coherent desired tree exists to diff; every other
   refusal publishes the computed diff annotated with the offending objects.
4. **Publish** the diff (or the refusal report). Nothing is applied in the reconcile that
   computed it.
5. **Requeue.** Whenever a pending diff, a deferral, or an in-progress transition exists, the
   reconcile returns a `RequeueAfter` and status reports the next attempt time. Nothing in this
   design waits for an unrelated event to make progress: the CephCluster reconcile does not
   requeue on success today, no manager `SyncPeriod` is configured, node-label events fire once,
   and the feature's own ConfigMap writes are filtered out of the watch.
6. **Apply, in the staged order below**, only when a recomputation at apply time — against a
   fresh read taken under the cluster's lock — yields the same diff as published. A fleet
   relabel in progress keeps moving the target, so convergence waits for labels to settle; and a
   host bucket created by a node that joined between publish and apply invalidates the published
   removal set rather than being silently swept away.

### The transition sequence

A transition has three stages, each individually safe, ordered so that no stage depends on a gate
its predecessor just closed:

1. **Type table first.** If the declared list requires new types or renumbering, apply a map
   write that changes *only* the type section. Bucket membership does not change and every
   in-map reference rebinds by name. (Placement is not *provably* unperturbed — the text round
   trip shifts a fraction of item weights by one 1/65536 unit, per Bucket identity — but the
   effect is bounded, self-limiting, and orders of magnitude below one PG on ordinary fleets.)
2. **Deployments second, on a healthy cluster.** Re-render every OSD Deployment's
   `--crush-location` and `topology-location-*` labels from the declared list, through the
   existing OSD updater with its bounded parallelism and `ok-to-stop` gating. For existing OSDs
   the new argument is inert against the map — `check_item_loc()` short-circuits at the host
   bucket — so this stage restarts OSDs but moves no data. What it buys is correctness of the
   Deployment labels and of any future daemon registration *before* the rebalance starts. The
   rendered location must be byte-identical to what the prepare job produces for the same node,
   so an already-converged Deployment yields no change and no restart.
3. **The bucket write last.** One atomic map write converges the hierarchy: creates, renames
   (id-preserving), re-parents, removals. CRUSH never sees an intermediate tree, one planned
   rebalance follows, and the Deployments already agree with the result.

The reverse order — map first, restarts second — deadlocks: the map write starts a fleet-wide
backfill, and the OSD updater's gates (the cluster-clean check when `upgradeOSDRequiresHealthyPGs`
is set, *and* `ok-to-stop` — they are cumulative, not alternatives) then block every restart
until the rebalance drains, aborting the OSD orchestration on its 20-minute no-progress limit and
leaving the CephCluster Failing for the duration.

**The stage 2→3 window is a known hazard and is bounded deliberately.** Between the Deployment
wave and the bucket write, the `topology-location-<fd>` labels name buckets the live map does not
yet contain. If every Rook pool CR declares a failure domain above `host`, a node drain in that
window drives the disruption controller's `noout` path — `ceph osd set-group noout <name>` on the
label-derived name — into `-EINVAL`, which today is logged and not returned, and the guard that
skips redundant sets never trips, so the failure repeats silently while the drained OSDs get
marked out and an unplanned rebalance starts. Two requirements follow: the disruption
controller's `noout` path must resolve the failure-domain name against the live map and
skip-with-report when it does not exist; and stage 3 must follow stage 2 promptly — the window is
otherwise held open by any refusal that latches after stage 2, because refusals freeze this
feature's writes without reverting stages already applied.

Stage state — which stage the transition is in, the last-applied declared list, the rendering
version, and the last type-id assignment — is persisted in the feature's ConfigMap, so an
operator restart resumes rather than restarts. An interrupted stage-2 wave continues to
completion even if the mode is disabled mid-wave — the exception stated in the CRD godoc — so
the fleet never lingers split between two renderings until some unrelated reconcile happens by;
rendering from the persisted list means it would eventually converge anyway, but "eventually" is
unbounded without the carve-out.

### Applying the map write safely

The write uses the same primitives as Rook's existing rule injection — `osd getcrushmap`,
`crushtool --decompile`, `crushtool --compile`, and `injectCRUSHMap`'s `osd setcrushmap` — but
not its editing model: the existing `updateCrushMap()` only ever *appends* rule text, and its two
callers (stretch and hybrid rules) never parse the decompiled text. Editing the bucket and type
sections while provably leaving rules and tunables untouched is new code with its own tests.

Two guards close the read-modify-write race, and both are scoped so that **CephClusters never
serialize against each other** — the race exists only within one Ceph cluster, so process-wide
exclusion would be pure waste:

- **In-process, per cluster:** a lock keyed by cluster namespace. The normalizer holds its
  cluster's write lock only across each read-modify-write — never across publishes, requeues, or
  stage 2. The existing whole-map read-modify-write paths take the same per-cluster lock: the
  rule-injection bodies (the outer sections currently under the package-global
  `crushRuleMutex.RLock()` in `pool.go`) and `CreateDefaultStretchCrushRule` in `mon.go`, which
  today reaches `updateCrushMap` holding **no** lock at all. The lock must not be taken inside
  the nested `updateCrushMap` itself — it is called from within the rule paths' guarded bodies,
  and Go's `RWMutex` is neither reentrant nor upgradeable, so locking there self-deadlocks.
- **Against out-of-process writers:** every `setcrushmap` — the normalizer's *and* the existing
  `injectCRUSHMap` callers', which today pass nothing — supplies `prior_version` with the
  **`crush_version`** read at the start of the RMW. `crush_version` is not the OSDMap epoch (the
  mon compares `osdmap.get_crush_version()`, which increments only on CRUSH changes); passing
  the epoch would `-EPERM` on essentially every write. When the argument is absent the mon skips
  the check entirely and the write lands unconditionally, so the implementation must assert it is
  always supplied. On `-EPERM` the pass abandons the write and recomputes. One mon nuance: a
  resend where `prior_version == current-1` and the proposed map is byte-identical returns
  success (idempotent-resend window) — a retry loop must treat that as its own write having
  landed, not as a conflict.

The in-process lock alone is not redundant with the CAS, and the CAS alone is not sufficient:
`prior_version` protects only the writer that passes it, so an un-guarded rule-path RMW reading
before and writing after the normalizer would still silently revert the hierarchy — a valid map,
so nothing errors — which is why both rule paths move under the per-cluster lock.

**`choose_args` (crush-compat balancer weight-sets).** The decompiled text carries them, keyed by
bucket id, and `crushtool --compile` hard-requires each weight array's length to equal its
bucket's current size — so a bucket-membership edit that leaves them untouched fails to compile,
and a preserved-but-stale weight-set that did compile would weight placement by undeclared
numbers. Normalization on a map whose `choose_args` intersect the diff is refused in v1
(refusal 11); regenerating weight-sets is future work.

### Refusals

All-or-nothing: any refusal blocks the entire normalization and freezes the feature's writes.
There is no partial mode, because normalizing around an exception produces a tree that matches
neither the labels nor the previous map. Wholesale refusal is deliberately the strict starting
point; loosening it for large fleets is future work if operating experience demands it.

Refused, with each reason reported and the offending object named:

1. **A node with Rook OSDs missing any declared label**, or carrying an empty value for one.
2. **A malformed or unsatisfiable declared list.** Two declared keys sharing a type suffix; a
   suffix colliding with the reserved types `osd`, `host`, or `root`; or buckets of an
   undeclared type existing under a *different* CRUSH root, which the renumbering cannot
   guarantee ascending-id parentage for.
3. **Duplicate or colliding names.** Two nodes whose values produce the same bucket name at
   different levels or under different parents. Collisions are detected on **normalized** values
   (dot→dash), because two raw values can collide only after normalization; and the check
   replaces — never inherits — the existing extractor's behavior of dropping a higher level
   whose raw value equals a lower one's with only a log warning. Enforcing this at diff time is
   for diagnosis quality: `crushtool --compile` would reject the duplicate anyway, but with an
   error naming no node.
4. **A CRUSH rule step that takes a bucket the normalization would remove *or rename*.** Take
   targets are compared after splitting any `~<class>` suffix, and the removal set includes the
   shadow names derived from every removed bucket. Renames are included because `step take`
   binds by name in the decompiled text and no numeric form parses — renaming a taken bucket
   while leaving the rule alone makes the compile fail with an error naming no cause, which is
   neither an apply nor a diagnosed refusal. The report names the pool pinning the bucket.
5. **A declared-order change that no tree can satisfy.** Reordering the list so that an existing
   level's values would need the same bucket name under multiple parents is reported against the
   **list**, not the nodes: no relabelling of nodes can clear it. Rook does not compute or
   suggest the value renames that would make the requested order representable; it logs, in
   detail, what the conflict is and which nodes' values are involved, and leaves the resolution
   to the administrator.
6. **Unmanaged items**: OSDs in the tree not owned by this cluster's Deployments, or buckets
   containing them. Removing or re-parenting them is not Rook's call.
7. **A stale owned host bucket**: a host bucket whose OSDs are all owned by this cluster's
   Deployments but whose node object no longer exists. Rook deliberately retains OSD Deployments
   for absent nodes (the updater's data-loss guard), and only the purge path removes host
   buckets, so this state is *expected* after any node loss — and normalization cannot place a
   host that has no labels. The report names the exact remedy: restore the node object, or purge
   the OSDs with the documented osd-purge job. It is never auto-exempted: exempting it would put
   the bucket in the removal set of an atomic map write, deleting placement for OSDs whose disks
   still hold data. (The node watch must also wake the reconcile on node *deletion* while the
   mode is set — today's predicate ignores deletes — or the transition into this refusal goes
   unreported until an unrelated event.)
8. **Portable PVC-backed OSDs present.** Their host buckets are named after PVCs, not nodes, so
   no node label describes them; a normalized tree can neither place nor preserve them
   coherently.
9. **Stretch clusters and per-pool stretch pools.** Cluster stretch mode is engaged against a
   generated rule and a failure-domain type. Independently, `osd pool stretch set` (present at
   v19.2.2) writes a raw CRUSH **type id** into `pg_pool_t::peering_crush_bucket_barrier`
   *without* engaging cluster stretch mode, and nothing revalidates it when the type table
   changes — a renumbering would silently repoint the pool's peering barrier. Any pool reporting
   `peering_crush_bucket_count != 0` in `osd pool ls detail` is refused as the same class.
10. **Placement infeasibility, checked by Ceph's own mapper.** For each pool: compile the
    desired map locally and run
    `crushtool -i <map> --test --rule <id> --num-rep <size> --pool-id <id> --max-x <pg_num-1>
    --show-bad-mappings`, refusing on any `bad mapping` line. This is the predicate Ceph's mon
    itself applies as an admission gate on pool creation and on `setcrushmap`, with `num_rep`
    from the pool's `size` — which for erasure-coded pools is `k+m`. It handles EC `indep`, MSR
    rules, multi-take, and device-class shadow trees natively, with no Rook-side rule parser —
    Rook's existing parser reads only two steps and cannot express an MSR rule at all, so an
    analytic walk would either be a large new parser or a permanent refusal on any cluster using
    one. Pool facts come from `osd pool ls detail`, never from Rook's CRs alone (which see only
    CephBlockPool, CephFilesystem, CephObjectStore, and CephObjectZone — not CephNFS, `.mgr`, or
    directly created pools). Two stated costs: `crushtool --test` exits 0 even when it reports
    bad mappings, so the check is a stdout scrape for the `bad mapping` prefix; and the tester
    weights every in-map device uniformly, so it answers "adequate assuming all OSDs in" — the
    same idealization an analytic count would make.
11. **Balancer weight-sets intersecting the diff** (`choose_args`, above).
12. **Cluster not healthy at apply time**, by PG state counters: `degraded`, `undersized`,
    `unfound`, or `inactive` PGs defer the apply — reported as a deferral, not a configuration
    refusal. One exemption: the apply is allowed despite unhealthy PGs when the **live** map
    fails refusal 10's placement check for at least one pool while the **desired** map passes it
    for every pool — the case where the current tree is itself the cause of the damage and
    converging is the only repair. (Evaluated per rule with the same `crushtool --test`
    machinery, so class-scoped pools are measured through their own shadow subtrees rather than
    over-counted on the plain tree.) The predicate is feature-owned rather than an extension of
    `spec.disruptionManagement.pgHealthyRegex`: that regex is a positive allow-list over
    compound state names, and composing movement states onto an operator-supplied pattern cannot
    express this check.

### Deployments, daemons, and the disruption controller

**Once a normalization has been applied**, the OSD Deployment's `--crush-location` argument and
`topology-location-*` labels render from **the last-applied declared list** — persisted in the
feature's ConfigMap — not from the Deployment's previous argument (today's self-perpetuating
behavior), not from the live map, and not from the CR field directly. Until a first normalization
applies — dry-run, or armed but still refused — rendering is unchanged from today: the built-in
label set. Stated consequence: during that never-applied window, a newly provisioned node still
renders from the built-in set, and its daemon may create ancestry for undeclared levels
(including CCM-applied zone/region); that is the bounded drift the Goals disclose, and the next
diff reports it.

Rendering from the persisted list is what makes **disarm** safe with custom label keys: the
built-in extractor cannot render them, so a disarmed operator that fell back to today's fixed-set
rendering would place the next added node outside the custom levels — its PGs unplaceable by any
rule choosing at those levels, surfacing as undersized PGs and `HEALTH_WARN` (CRUSH drops the
replica slot; this failure is loud, not silent) — and, whenever a pool's failure domain is a
built-in-named type supplied by a custom key, as a namespace-wide PDB reconcile failure. The
persisted list keeps every future node, disk replacement, and prepare job consistent with the
normalized map whether or not the mode is still armed. The prepare job receives the list the way
it receives per-job configuration today (its own environment, like `ROOK_CRUSHMAP_HOSTNAME` —
not the shared `getConfigEnvVars` channel, which also feeds daemon pods and would restart them on
change).

**A Rook downgrade is the one lifecycle event this cannot cover, and it is an unmitigated
hazard.** An older operator binary has no code that reads the feature's ConfigMap, so after a
downgrade the persisted list is inert: existing OSDs keep their baked arguments, but the next
node added (or OSD re-provisioned) renders from the built-in label set, with the consequences
above. The CR field is also pruned silently by the older schema, so re-upgrading requires
re-arming with no signal that the grant was dropped. The documentation must state a pre-downgrade
procedure for clusters that adopted custom label keys; there is no in-design mitigation.

Consequences, stated plainly:

- For existing OSDs the re-rendered argument is inert against the map — the `check_item_loc()`
  short-circuit — so its payload is the Deployment labels and future daemon registrations, not
  placement. The restart it costs is why stage 2 runs on a healthy cluster.
- The `topology-location-*` labels are load-bearing: the disruption controller enumerates
  failure domains from Deployment labels, its blocking PDBs select **pods** by the same key, and
  a Deployment missing the pool's failure-domain label fails PDB reconciliation for the whole
  namespace. Staging Deployments before the map write keeps them correct throughout (the bounded
  stage 2→3 window excepted, above).
- Custom levels become usable as pool failure domains: `pkg/operator/ceph/pool/validate.go`
  checks `failureDomain` against the live type table, so a custom type validates once stage 1
  has run. The disruption controller's failure-domain ranking (`getMinimumFailureDomain`) orders
  levels against Rook's hardcoded built-in list and silently falls back to `host` for anything
  else; while the mode is set, that ranking must use the declared order, and the switch is gated
  on stage 2 having completed, so the ranking never demands a label the Deployments do not yet
  carry. (The hardcoded ranking also has a standing off-by-one that makes `region` unreachable —
  pre-existing, tracked separately.)
- **CSI read affinity follows the declared list, gated the same way.** Both Rook code paths
  substitute the built-in default label set into `csi.readAffinity.crushLocationLabels` whenever
  that field is empty; once stage 2 has completed, the substituted default becomes the hostname
  label plus the declared list, so read-affinity hints describe the map that exists. The switch
  is deliberately **not** keyed on the mode being set — that would rewrite live CSI
  configuration from the dry-run state the table promises writes nothing. The effective CSI
  label set appears in the published diff. An explicitly configured CSI field is honored as-is.
- A restarting OSD whose PGs are unclean latches the disruption controller's active-drain
  handling — the trigger is Deployment `ReadyReplicas < 1` plus unclean PGs, no node drain
  required — which deletes the default OSD PDB and creates `maxUnavailable=0` blocking PDBs on
  every other failure domain. Stage 2's restarts carry a **feature-owned exemption marker**,
  honored by the disruption controller's exemption check alongside the existing one. It must not
  reuse `osd.rook.io/replace-in-progress`: that annotation is the OSD replacement state
  machine's ownership marker, and setting it outside that flow makes the next health tick
  *cancel* a phantom replacement per OSD — `ceph osd in`, replicas forced to 1, and the
  skip-reconcile fence deleted. The marker is set on **the updater-selected `ok-to-stop`
  batch** — at most `osdMaxUpdatesInParallel` OSDs, since per-OSD marking is not expressible
  through the updater, which selects and updates batches together — applied immediately before
  the batch update, removed on readiness, with a sweep clearing markers orphaned by an operator
  restart. It is never fleet-wide: a fleet-wide exemption blinds down-detection entirely. (An
  adjacent, pre-existing gap worth naming: today's ordinary OSD update waves restart the same
  batches with *no* exemption and already trip the drain latch when PGs are unclean; this design
  neither causes nor fixes that.)
- **The declared list supersedes the built-in topology validation — once OSDs exist.** While the
  mode is set and the cluster has at least one OSD, the legacy validator
  (`topology.CheckTopologyConflicts`, which reasons over the built-in label set, latches
  process-globally after one clean pass, downgrades conflicts to a warning once OSDs exist, and
  is bypassable by `ROOK_SKIP_OSD_TOPOLOGY_CHECK`) is not run for this cluster: refusal classes
  1–3 replace it, evaluated over the declared keys on every reconcile with none of those escape
  hatches. On a cluster with **zero** OSDs the legacy behavior is kept: declared-list validation
  failures surface as a reconcile error that blocks OSD provisioning, exactly as the legacy
  validator does today — refusals cannot serve that role there (they are scoped to nodes with
  Rook OSDs, vacuous on an empty cluster, and deliberately surface no error), and letting OSDs
  provision into a refused topology is the strictly worse outcome. Clusters that never set the
  mode keep the legacy validator unchanged.

### Continuous semantics

While confirmed, the mode holds the hierarchy to the labels:

- **Manual `ceph osd crush move` or `add-bucket` beneath the root is reverted** on the next
  applied pass. This is the declarative contract, it is the point of the feature, and it is
  stated in the CRD godoc because it is also the sharpest behavior change for anyone used to
  hand-editing the map. There is deliberately no pause short of clearing `Confirmation`; an
  emergency that requires manual CRUSH surgery is an emergency in which the labels should be
  updated instead, and the publish-then-apply cycle gives the operator the window to do so.
- A label edit is the API for changing topology. The node watch wakes the reconcile — noting
  that the existing predicate compares a fixed, package-initialized label list and ignores node
  deletion, so while the mode is set the watch must cover the declared keys and node deletes
  instead.
- Disabling the mode stops map writes and reverts nothing. Rendering continues from the
  last-applied list (above), so the cluster stays internally consistent; what is lost is
  convergence. Re-arming resumes it. A Rook **downgrade** does not behave like disabling — see
  the downgrade hazard above.

### Reporting

- The full diff — every create, rename, re-parent, and removal; every per-OSD location change;
  the effective CSI label set; every refusal with its reason and offending object; and the stage
  of any in-progress transition — is written to the feature's ConfigMap. Status carries an
  aggregated summary (counts and a bounded sample per category) because a per-bucket list on
  status is unbounded on a large fleet, and events are garbage-collected on a TTL outside Rook's
  control.
- Deferrals (unhealthy PGs, unsettled labels) are distinguished from refusals (configuration
  contradicts the model) in both status and the ConfigMap, and status names the next retry time.
- Every refusal is logged in detail — the problem and the node(s) or object involved — and
  emits a Kubernetes Event on the CephCluster. Two mechanics matter: the events/v1 series cache
  keys on `{type, action, reason, controller, instance, object}` and never updates a series'
  message, so a parked refusal re-emitted faster than the ~6-minute series window would keep
  naming the *first* offender forever. Therefore the offender-set identity (a hash) is folded
  into the event `action`, so a changed offender set starts a new series; the status condition
  is rewritten every pass; and the ConfigMap is the authoritative record.

### Testing

Unit coverage over fixture CRUSH maps and node lists exercises the pure function: desired-tree
computation for every declared-list shape, including custom keys, mixed built-in/custom
hierarchies, and type-table renumbering with undeclared built-in types; identity matching by
child-id set, including in-place renames and rename-plus-membership-change in one window; each
refusal class, including `~`-suffixed take targets, renamed take targets, per-pool stretch
pools, and the reorder refusal; the diff against maps with stale, misplaced, and undeclared
buckets; collision detection on normalized values; and idempotence — normalizing a normalized
map yields an empty diff, and re-rendering a converged Deployment yields no change.

**Byte-stable rendering is enforced by a golden fixture**, embedded via `go:embed` (the repo's
one existing fixture-directory precedent is `pkg/operator/ceph/file/mds/test/`): rendered output
for a set of declared-list shapes is pinned, and regenerating the fixture is treated as a design
change, not a test chore — it is what keeps the persisted rendering-algorithm version honest.
The upgrade suite can additionally chain N-1→N stability once the base release carries the
feature, but only over a one-host hierarchy, so it is a supplement, not the mechanism.

**A multi-node integration job is a prerequisite, not follow-up work** — the design's
load-bearing Ceph claims are read from v19.2.2 source, not observed on a running cluster, and
this feature replaces whole CRUSH maps. It is also close to a from-scratch build, priced
honestly: Rook's integration kind cluster is single-node by design (its config bind-mounts the
host's `/dev`, `/var/lib/rook`, and `/run/udev`, so added worker nodes would share one device
inventory and one `dataDirHostPath`), the only multi-node kind config in the repo deploys no
Ceph, and `tests/scripts/localPathPV.sh` is single-node by construction — its node selection
breaks outright on multiple nodes, every PV it creates shares one node-affinity label, and its
mon directories live under one host path. The job therefore needs a new Ceph-capable multi-node
kind config with per-node mounts and a rewritten local-PV script; `github-action-helper.sh`'s
node loops are already multi-node-ready and can be reused. Capacity is unvalidated: every Ceph
workflow runs on standard `ubuntu-22.04` runners (4 vCPU / 16 GB, disk already trimmed, swap
disabled), and a multi-node kind cluster plus Ceph on that class has not been demonstrated.
The job uses **non-portable PVC-backed OSDs on per-node local PVs**; they resolve their CRUSH
host to the node's hostname and are full participants in normalization, so it exercises the
staged transition, bucket removal, custom types, and the restart path. Two things it structurally
cannot exercise — the raw-device prepare path, and the disruption-controller interaction (the
replacement processor that made the exemption marker necessary early-returns on PVC-backed
clusters) — stay covered by unit tests, and the PDB interaction needs a host-based test variant
before the feature leaves experimental status.

## Risks and Mitigation

**Bucket removal at fleet scale is new.** Mitigated by: the required, defaultless declared list;
the dry-run state showing every removal and every per-OSD change before arming; refusal classes
4, 6, 7, 10, and 11 protecting rule targets, unmanaged items, stale owned hosts, pool placement,
and balancer state; and atomicity, which makes the transition reviewable as a single diff.

**Fleet-wide first transition.** On an existing cluster, first confirmation typically rewrites
the type table (no membership change), restarts every OSD (stage 2, healthy cluster, no data
movement), and then moves most data once (stage 3). This is disclosed as the cost of adopting
the model. It compares favorably with the documented node-by-node re-add, which moves roughly
twice the data at reduced redundancy per node and removes no stale buckets.

**Broad-RBAC label edits drive restructuring while armed.** Anyone who can label nodes can
reshape (and, by removing labels — refusal 1 — halt) the hierarchy. Mitigated by the explicit
standing grant, publish-then-apply with settle, and refusal 1 turning a deleted label into a stop
rather than a removal. Not mitigated further: per-edit confirmation is incoherent for a
continuous reconciler, and the doc says so rather than pretending otherwise.

**A wrong declared list.** Declaring too few levels removes real structure — that is the feature
working as specified, surfaced in the dry-run diff. Declaring a level whose labels are absent
from any node is refusal 1. The dry-run state is the mitigation, and the documentation must
instruct reviewing it before ever setting the confirmation.

**Ordinary node loss parks convergence.** Refusal 7 fires whenever a node object disappears while
its OSDs remain — the expected state after unpurged node loss on any long-lived cluster. The
cluster itself is unaffected (the map simply stops converging), and the report names the remedy;
but operators must know that node hygiene is now part of keeping the feature converging.

**Operator upgrade.** A cluster that never sets `crushTopology` is untouched. An armed cluster
must see an empty diff across the upgrade, enforced by the golden fixture and the persisted
rendering version (Testing).

**Operator downgrade.** Unmitigated for clusters using custom label keys — see the downgrade
hazard in the Deployments section. The documentation must carry the pre-downgrade procedure.

## Drawbacks

This is a mode switch, not an enhancement: hand-built CRUSH structure beneath the root and this
feature are mutually exclusive, and adopting it on a brownfield cluster is a planned migration
event, not a tweak.

Ceph's own orchestrator went the other way — cephadm's `HostSpec.location` is *actuated* only at
host-add (re-adding a host short-circuits mon-side on the existing bucket name; the field is
otherwise only persisted and displayed). Rook choosing continuous, removal-inclusive
reconciliation is a deliberate divergence and should be argued on review. The argument for it: a
half-owned hierarchy has no stable boundary between Rook's structure and the administrator's, and
Kubernetes operators exist to make declared state true.

Rook also takes ownership of the CRUSH type table, including renumbering it. That is a stronger
claim on the map than any Rook feature has made, and it is forced by Ceph's location-walk
semantics rather than chosen for convenience; the alternative — restricting custom levels to ids
above the built-ins — silently breaks daemon-side ancestry construction and subtree health
checks.

`check_item_loc()`'s early return remains load-bearing for existing OSDs and should **not** be
"fixed" upstream: it is what makes stage 2's restarts placement-inert and stops every OSD restart
from re-asserting a stale daemon-side location against the normalized map.

## Alternatives

**Merge with preserved hand-built structure.** Manage the labelled levels while preserving
buckets the labels do not describe. Considered at length and rejected: any such design needs a
per-host rule for where managed structure ends and preserved structure begins, and every
candidate rule either refuses ordinary operations (re-racking across a preserved level, inserting
a level above an existing one) or produces order-dependent trees on partially labelled fleets.
The defects are artifacts of holding two sources of truth; this model holds one, at the cost of
the migration event above.

**Report-only as the whole feature.** Detect and report every divergence and leave the
administrator to run the commands. Rejected: it leaves fleet-scale CRUSH surgery in human hands,
which is the work an operator exists to automate. The dry-run state is this alternative's useful
half, kept as a state of the real feature — the same code path stopping short of the apply.

**Incremental convergence (one host per reconcile).** Rejected for normalization: intermediate
trees are hierarchies nobody declared, they pass through reduced-failure-domain states the atomic
write avoids, and each per-host `osd crush move` is a separate mon proposal with no transaction
around the sequence.

**Map-first, restarts-second.** Rejected: the map write closes the gates the restarts need (see
The transition sequence).

**An analytic rule parser for placement checking.** Rejected in favor of `crushtool --test`
(refusal 10): Ceph's own mapper is the only complete rule implementation, the mon already uses
this predicate as an admission gate, and an analytic walk over the shadow-free tree is
structurally blind to class-scoped placement.

**Node re-add workaround, documented better.** Costs a full backfill out and back per node and
removes no stale buckets.

## Open Questions

1. **Should the built-in topology label support be deprecated?** Today's fixed-set behavior —
   `topology.rook.io/*` plus the Kubernetes zone/region labels, applied only at first OSD
   creation — becomes a legacy second placement engine beside this mode. Deprecating it
   (declare-or-nothing) would collapse the two engines into one, retire the latching validator
   and its `ROOK_SKIP_OSD_TOPOLOGY_CHECK` bypass, and make the declared list the single
   vocabulary. But it forces every labelled cluster through an eventual adoption event, and it
   turns CCM-applied zone/region labels from default behavior into inert-unless-declared — a
   behavior change for clusters that never opt in.
2. **Is wholesale refusal workable on large fleets in practice**, or does one perpetually
   mislabelled node hold the entire hierarchy hostage? Strict is the deliberate starting point;
   the loosening, if ever, is per-refusal-class reporting with an explicit allowlist, not silent
   partial normalization.
