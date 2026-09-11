---
title: osd-crush-topology-reconciliation
target-version: release-1.21
---

# OSD CRUSH Topology Normalization

## Summary

Rook reads a node's topology labels once per OSD, in the OSD prepare job, and bakes the result
into the OSD Deployment as a `ceph-osd --crush-location=` argument. Changing a topology label on a
node that already hosts OSDs has no online effect on the CRUSH map: Ceph's own location check
(`check_item_loc()`) stops at the host bucket, so nothing above `host` is ever examined for an
existing OSD, and the hierarchy drifts permanently away from what the labels describe. The
documented remedy is to remove each node from the cluster and re-add it — a full backfill out and
back per node — and it removes no stale intermediate buckets.

This proposal adds an opt-in, continuously reconciled mode in which **node labels are the only
source of truth for the CRUSH bucket hierarchy beneath the configured root**. While enabled, Rook
computes the tree the labels describe, diffs it against the live map, and converges the map to it
in a single atomic operation — creating, moving, and **removing** buckets as required. Structure
the labels do not describe is not preserved; it is what normalization removes.

The primary value is not the one-time repair of a drifted hierarchy — a one-time change can
always be made from the toolbox with `ceph osd crush move`. It is the standing ownership: while
the mode is enabled, Rook owns the CRUSH topology, node labels are the only interface for
changing it, and manual map edits stop being a parallel — and permanently divergent — channel.
Drift repair falls out of that contract; it is not the feature.

Rook already automates CRUSH-mutating operations of this class behind opt-in, default-off flags:
`allowDeviceClassUpdate` and `allowOsdCrushWeightUpdate` both cause data movement when enabled and
are off by default for exactly that reason. This mode follows the same pattern, with a stronger
gate: a dry-run state that publishes the complete diff, and an explicit confirmation string in the
`yes-really-*` family before anything is written.

## Goals

- While the mode is enabled, the hierarchy beneath the configured root is a pure function of node
  labels: buckets the labels describe exist and are correctly parented; buckets they do not
  describe are removed.
- The map transition is atomic — one mon operation, no intermediate hierarchy, one planned
  rebalance.
- Anything that prevents a safe, complete normalization refuses the whole operation and reports
  precisely why, without disturbing any other part of cluster reconciliation. A refusal freezes
  this feature's own writes; it does not freeze OSD provisioning, so the map may still gain (never
  lose) buckets during a refusal, and the next diff reports them.
- The full desired-vs-actual diff — including per-Deployment changes — is visible before anything
  is modified, and durable.
- Default to today's behavior; nothing changes until the mode is explicitly enabled.
- Nothing serializes reconciliation across CephClusters: every exclusion in this design is scoped
  to a single cluster.

## Non-Goals

- **Merging with hand-built structure.** There is no mode that preserves buckets the labels do
  not describe while managing the rest (see Alternatives). An administrator who wants hand-built
  structure keeps the feature off.
- **Removing OSDs or hosts.** OSD and host lifecycle remain the provisioning and purge paths'
  job; normalization restructures only the hierarchy above the host level.
- **Device classes.** Ceph derives the `~<class>` shadow hierarchies itself; they never appear in
  the decompiled map text, so the diff cannot touch them. Rules referencing them are covered by
  the refusal checks.
- **CRUSH rules and tunables.** Normalization edits the bucket and type sections only. A rule
  that references a bucket normalization would remove or rename is a refusal, not an edit.
- **Multiple CRUSH roots.** Only the tree beneath `spec.storage.config.crushRoot` is normalized;
  other roots are untouched (one narrow type-table interaction is refusal 2).

## Design

### The model

```text
desired tree = f(crushRoot, declared label list, nodes with Rook OSDs)
```

The declared label list defines which CRUSH levels exist and their order, highest first. For each
node hosting Rook OSDs, the node's values for the declared labels define the chain of buckets
from the root down to its host bucket. Host bucket names derive from the node as today. Any
bucket beneath the root that is not part of the computed tree is removed.

Because desired state is a pure function of labels, there is no anchor, no per-host placement
walk, and no merge logic — the questions that sink a merge design (where does managed structure
end?) are never asked.

**Bucket identity.** Desired and actual trees are matched bottom-up: host buckets by their stable
node-derived names, each level above by (type, child-**id** set). Matched buckets keep their
CRUSH id; a matched bucket whose name differs is renamed in place, so fixing a typo in a rack
name moves no data — unless a rule takes the old name, which is a refusal. Item weights survive
the map rewrite to within one 1/65536 unit in the ordinary weight range, a few units on very
large aggregates (bit-for-bit is impossible through the
`crushtool` text round trip; the drift is bounded, self-limiting, and does not accumulate because
weights never enter the diff — a pass with no structural change performs no write).

The desired-tree rendering must be **byte-stable across Rook releases**: an armed cluster must
not see a rebalance because the operator image was upgraded. A golden test fixture pins the
rendered output, and a rendering-algorithm version persisted with the last-applied list turns any
mismatch into a refusal requiring a fresh dry-run review.

### The type table

The declared list may contain **any** label keys, not only Rook's built-in topology labels; each
key's suffix after the last `/` names its CRUSH type. Ceph's mon- and daemon-side location walks
treat ascending type-id order as the hierarchy (verified against v19.2.2 source), so simply
appending new types breaks new-host ancestry construction and subtree health reporting. The
normalization therefore **owns the type table** and renumbers it so id order matches the declared
order. This is safe inside the map, where the decompiled text binds types by name; it is not safe
for the raw type ids some pool-level stretch settings store outside the map, which is why those
are a refusal. Built-in types with no remaining buckets are dropped; external tooling that cached
raw type ids observes the renumbering, which is disclosed as a consequence.

### API

Two fields on `spec.storage`, under a new optional `crushTopology` object:

- `crushLocationLabels` (required, no default): the ordered list of full label keys, highest
  level first; the host level is implicit and lowest. Required-with-no-default is deliberate:
  cloud controller managers apply `topology.kubernetes.io/zone`/`region` without administrator
  action, so any default would let platform-applied labels define — and their absence remove —
  CRUSH structure nobody consciously declared.
- `confirmation` (optional): must equal `yes-really-normalize-crush-topology` for Rook to write
  anything, following the `yes-really-*` precedent (`Migration.Confirmation`,
  `OSDStore.UpdateStore`). A string rather than a boolean because arming is a standing grant:
  while armed, node-label edits — whose RBAC is typically broader than CephCluster edit rights —
  drive restructuring. Per-plan approval is deliberately not offered; it is incoherent for a
  continuous reconciler, and the dry-run state exists so the first transition is reviewed before
  the grant.

| State | Spec | What Rook does |
|---|---|---|
| **Off** | `crushTopology` absent | Nothing computed. A cluster that previously applied a normalization keeps rendering OSD locations from the last-applied list (below); any other cluster: today's behavior exactly. |
| **Dry-run** | list set, `confirmation` empty | Computes and publishes the full diff — creates, moves, removals, per-OSD location changes, refusals — every reconcile. **Writes nothing to Ceph or to any Deployment.** |
| **Armed** | list set, `confirmation` exact | Continuous normalization: the staged transition runs, then the hierarchy is held to the labels. Every later label edit self-applies once it settles. |

Clearing the confirmation returns to dry-run: writes stop (with one exception — a Deployment
re-render wave already in flight completes against the last-applied list, so the fleet is never
left split between two renderings) and nothing is reverted. Exact validation markers follow CRD
conventions and `kube-api-lint`; they are implementation detail.

### Example

```yaml
spec:
  storage:
    config:
      crushRoot: default # optional; "default" if unset
    crushTopology:
      confirmation: yes-really-normalize-crush-topology # empty/omitted = dry-run
      crushLocationLabels:
        - topology.kubernetes.io/zone # highest declared level
        - example.com/hall            # custom key -> CRUSH type "hall"
        - topology.rook.io/rack       # lowest declared level; host is implicit
```

With `node-a` labelled `zone=z1`, `hall=hall2`, `rack=cab104`, the normalized tree is
`root default → zone z1 → hall hall2 → rack cab104 → host node-a → osd.N`. Any other bucket
beneath the root — a hand-built tier, a stale rack — is removed. A node with Rook OSDs missing
any declared label refuses the whole normalization, naming the node. Relabelling
`rack=cab105` later moves the host bucket on the next applied pass — the label edit is the API.

### How it converges

Each reconcile while the mode is set: read the map and cluster facts once under the cluster's
CRUSH lock; compute the desired tree, type table, and diff; check refusals; **publish** the diff
(or refusal report) durably; requeue. The apply happens on a later pass, and only when a fresh
recomputation under the lock yields the same diff as published — so a fleet relabel in progress
settles before anything moves, and a node that joined in between invalidates a stale removal set
instead of being swept away.

An apply is a three-stage transition, ordered so no stage depends on a gate its predecessor just
closed:

1. **Type table first** — a map write changing only the type section; membership is untouched.
2. **Deployments second, on a healthy cluster** — re-render every OSD Deployment's
   `--crush-location` and `topology-location-*` labels through the existing OSD updater. For
   existing OSDs the new argument is placement-inert (the `check_item_loc()` short-circuit), so
   this restarts OSDs but moves no data; it makes the Deployment labels correct before the
   rebalance.
3. **The bucket write last** — one atomic map write: creates, id-preserving renames, re-parents,
   removals. One planned rebalance follows, and the Deployments already agree with the result.

The reverse order deadlocks: a map-first write starts a backfill that closes the OSD updater's
health gates, stalling the restarts against a 20-minute abort. The stage 2→3 window — Deployment
labels naming buckets the map does not yet hold — is bounded deliberately: stage 3 follows
promptly, and the disruption controller's `noout` path must skip-with-report when a
failure-domain name does not resolve against the live map. Stage state persists in the feature's
ConfigMap so an operator restart resumes rather than restarts.

**Write safety.** The read-modify-write is guarded per cluster, never process-wide: a lock keyed
by cluster namespace, taken by the normalizer and by the existing whole-map writers (the rule
injection paths, one of which currently takes no lock). Against out-of-process writers, every
`setcrushmap` supplies `prior_version` with the CRUSH map version read at the start of the pass,
so a concurrent write yields a clean failure and a recompute instead of a silent revert. Balancer
weight-sets (`choose_args`) intersecting the diff are a refusal in v1: they are keyed by bucket
id with size-matched arrays, and regenerating them is future work.

### Refusals

All-or-nothing: any refusal blocks the entire normalization and freezes the feature's writes.
There is no partial mode, because normalizing around an exception produces a tree matching
neither the labels nor the previous map. Wholesale refusal is deliberately the strict starting
point; loosening is future work if operating experience demands it. Each refusal is reported with
the offending object named:

1. A node with Rook OSDs missing any declared label, or carrying an empty value.
2. A malformed or unsatisfiable declared list (duplicate type suffixes; collisions with
   `osd`/`host`/`root`; buckets of an undeclared type under a different CRUSH root; or a suffix
   or rendered bucket name that does not survive the `crushtool` text round trip unchanged — a
   mechanical check that covers CRUSH grammar keywords such as `rule`, `device`, `type`,
   `tunable`, and `choose_args`, and all-numeric names, without a hand-maintained denylist).
   Label-key charset needs no check: every valid Kubernetes label name is a valid CRUSH name.
3. Two nodes whose normalized values produce the same bucket name at different levels or under
   different parents.
4. A CRUSH rule step taking a bucket (or its device-class shadow) the normalization would remove
   or rename; the report names the pool pinning it.
5. A declared-order change no tree can satisfy — reported against the list, with the conflicting
   values; Rook does not compute value renames to resolve it.
6. Unmanaged items: OSDs not owned by this cluster's Deployments, or buckets containing them.
7. A stale owned host bucket — OSDs owned but the node object gone (expected after unpurged node
   loss). The report names the remedy: restore the node or purge the OSDs. Never auto-exempted:
   exemption would place the bucket in an atomic removal set, deleting placement for OSDs whose
   disks still hold data.
8. Portable PVC-backed OSDs present (their host buckets are named after PVCs, not nodes).
9. Stretch clusters and per-pool stretch pools (pool-level stretch stores a raw CRUSH type id
   outside the map that renumbering cannot rebind).
10. Placement infeasibility, checked for every pool in the cluster — not only CR-backed ones —
    with Ceph's own mapper (`crushtool --test` on the
    locally compiled desired map — the same predicate the mon applies as an admission gate; it
    handles EC, MSR, and device-class shadow trees natively, with no Rook-side rule parser).
11. Balancer weight-sets (`choose_args`) intersecting the diff.
12. Cluster not healthy at apply time (degraded/undersized/unfound/inactive PGs) — reported as a
    deferral, not a refusal; exempted only when the live map already fails the placement check
    and the desired map passes it, i.e. converging is the repair.

### Deployments and the disruption controller

Once a normalization has applied, OSD Deployments and prepare jobs render locations from **the
last-applied declared list**, persisted in the feature's ConfigMap — not from the Deployment's
previous argument, the live map, or the CR field. This is what makes disarm safe with custom
label keys: the built-in extractor cannot render them, and a fallback to today's fixed-set
rendering would place the next added node outside the custom levels. Consequences:

- The `topology-location-*` labels stay load-bearing for the disruption controller (failure
  domain enumeration, blocking-PDB selectors); staging Deployments before the map write keeps
  them correct.
- Custom levels become usable as pool failure domains; the disruption controller's
  failure-domain ranking uses the declared order while the mode is set, gated on stage 2 having
  completed. CSI read-affinity's substituted default label set follows the declared list under
  the same gate; an explicitly set CSI field is honored as-is.
- Stage 2's restarts carry a feature-owned drain-exemption marker on each updater batch, honored
  by the disruption controller and swept on operator restart. It must not reuse
  `osd.rook.io/replace-in-progress`, which drives the OSD replacement state machine.
- While the mode is set and OSDs exist, refusals 1–3 replace the legacy built-in topology
  validator for this cluster, with none of its escape hatches; a zero-OSD cluster keeps the
  legacy hard-block so a broken declared list still blocks initial provisioning.
- **A Rook downgrade is an unmitigated hazard for clusters using custom label keys**: an older
  operator cannot read the persisted list, the next added node renders from the built-in set,
  and the CR field is pruned silently. The documentation must carry a pre-downgrade procedure.

### Continuous semantics and reporting

Manual `ceph osd crush move`/`add-bucket` beneath the root is reverted on the next applied pass —
the declarative contract, stated in the CRD godoc, with no pause state short of clearing the
confirmation. A label edit is the API for changing topology; the node watch covers the declared
keys and node deletion while the mode is set. Disabling stops writes and reverts nothing.

The full diff, every refusal, and the transition stage are durable in the feature's ConfigMap;
status carries a bounded summary and the next retry time; every refusal logs in detail and emits
a Kubernetes Event on the CephCluster (with the offender set folded into the event action, so
event-series deduplication cannot pin a stale message).

### Testing

Unit fixtures over CRUSH maps and node lists cover the pure function: tree computation for
built-in, custom, and mixed label sets; type renumbering; identity matching and renames; every
refusal class; and idempotence (normalizing a normalized map yields an empty diff). A golden
fixture pins byte-stable rendering across releases. A **multi-node integration job is a
prerequisite, not follow-up work** — it needs a new Ceph-capable multi-node kind configuration
and local-PV tooling (the existing integration cluster is single-node by design, and CI runner
capacity for multi-node kind plus Ceph is undemonstrated), and it
exercises the staged transition, bucket removal, and custom types on PVC-backed OSDs; a
host-based variant covering the PDB interaction is required before the feature leaves
experimental status.

## Risks and Mitigation

- **Bucket removal at fleet scale is new.** Mitigated by the required defaultless list, the
  dry-run diff, the refusal classes, and atomicity.
- **Fleet-wide first transition.** First confirmation typically restarts every OSD (no data
  movement) and then moves most data once. Disclosed as the adoption cost; it compares favorably
  with the documented node-by-node re-add, which moves roughly twice the data at reduced
  redundancy and removes no stale buckets.
- **Broad-RBAC label edits drive restructuring while armed.** Mitigated by the explicit standing
  grant, publish-then-apply with settle, and refusal 1 turning a deleted label into a stop
  rather than a removal.
- **A wrong declared list removes real structure.** That is the feature working as specified;
  the dry-run diff is the mitigation, and the documentation must instruct reviewing it.
- **Ordinary node loss parks convergence** (refusal 7). The cluster is unaffected; the report
  names the remedy; node hygiene becomes part of keeping the feature converging.
- **Operator upgrade** must produce an empty diff on an armed cluster (golden fixture, rendering
  version). **Downgrade** is the unmitigated hazard above.

## Drawbacks

This is a mode switch, not an enhancement: hand-built structure and this feature are mutually
exclusive, and brownfield adoption is a planned migration event. Ceph's own orchestrator went the
other way — cephadm's `HostSpec.location` is actuated only at host-add — so continuous,
removal-inclusive reconciliation is a deliberate divergence: a half-owned hierarchy has no stable
boundary, and Kubernetes operators exist to make declared state true. Rook also takes ownership
of the CRUSH type table — a stronger claim on the map than any existing Rook feature makes,
forced by Ceph's location-walk semantics rather than chosen.

## Alternatives

- **Merge with preserved hand-built structure.** Rejected: every candidate boundary rule either
  refuses ordinary operations or produces order-dependent trees; the defects are artifacts of
  holding two sources of truth.
- **Report-only as the whole feature.** Rejected: it leaves fleet-scale CRUSH surgery in human
  hands, which is the work an operator exists to automate. The dry-run state is this
  alternative's useful half, kept as a state of the real feature.
- **Incremental convergence (one host per reconcile).** Rejected: intermediate trees nobody
  declared, reduced-failure-domain states, no transaction around the sequence.
- **An analytic rule parser for placement checking.** Rejected in favor of `crushtool --test`:
  Ceph's mapper is the only complete rule implementation and the mon already uses this predicate
  as an admission gate.
- **The node re-add workaround, documented better.** A full backfill per node, and it removes no
  stale buckets.

## Open Questions

1. **Should the built-in topology label support be deprecated?** Deprecating it would collapse
   Rook's two placement engines into one and retire the legacy validator, but forces every
   labelled cluster through an eventual adoption event and makes CCM-applied zone/region labels
   inert unless declared.
2. **Is wholesale refusal workable on large fleets**, or does one mislabelled node hold the
   hierarchy hostage? Strict is the deliberate start; the loosening, if ever, is per-class
   reporting with an explicit allowlist, not silent partial normalization.
