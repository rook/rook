# Object storage integration tests

This tree holds the new-style integration tests for rook's object storage
features. It is the model for converting the remaining old-style object tests
(`testObjectStoreOperations`, store lifecycle);
follow the conventions here exactly so the conversion happens once.

## Execution model

- One Kubernetes cluster is shared by the whole `TestCephObjectSuite` run. The
  testify suite in `tests/integration/ceph_object_test.go` owns cluster
  lifecycle and dispatches into the packages here; these packages are plain Go
  libraries, **not** standalone `go test` targets.
- A single shared `CephObjectStore` fixture (`util/sharedstore`) is created
  once per suite pass and torn down after all packages run. It provides the
  store CR, a go-ceph rgw admin client, and an SNS client, and is created with
  or without TLS to match the pass. The packages run in both passes: the
  with/without-TLS split runs them as separate parallel jobs, so the TLS pass
  adds no serial runtime to a single job.
- Packages run **sequentially** and each test is an ordered script of
  subtests. Do not use `t.Parallel`: the steps depend on prior steps, and the
  cluster is shared.

## Layout

The tree mirrors `pkg/operator/ceph/object/*`: one top-level directory per
operator package, with a feature subdirectory layer when multiple test
packages cover subsets of one operator package. The operator ROOT package
(the CephObjectStore controller itself) maps to the tree root,
`tests/integration/object/` itself, not a subdirectory. Leaf package names
must not collide with std-lib package names (no `io`, no `http`).

| test package | operator package | covers |
|---|---|---|
| `bucket/lifecycle` | `object/bucket` | OBC bucketLifecycle management |
| `bucket/owner` | `object/bucket` | OBC `bucketOwner` handling |
| `bucket/policy` | `object/bucket` | OBC bucketPolicy management |
| `bucket/quota` | `object/bucket` | OBC maxObjects user quota + bucketMaxObjects/bucketMaxSize bucket quota |
| `bucket/rw` | `object/bucket` | OBC S3 read/write/delete + OBC-stays-Bound |
| `cosi` | `object/cosi` | CephCOSIDriver + COSI bucket provisioning |
| `notification` | `object/notification` | CephBucketNotification HTTP endpoint delivery |
| `topic/kafka` | `object/topic` | CephBucketTopic kafka endpoints |
| `user/caps` | `object/user` | user capabilities |
| `user/keys` | `object/user` | explicit S3 key management |
| `user/opmask` | `object/user` | user op_mask |
| reserved: `tests/integration/object/lifecycle`, `tests/integration/object/dependents` | | future conversions |

Shared utilities live under `util/`:

- `wait4` — waits for object-test state: watch-based `Assert/Require` ×
  `Create/Delete/Condition/Absent` for k8s resources, `Assert/RequireEventually`
  polling for non-k8s state, `Assert/RequirePodLog` for a matching line in a
  pod's log stream, the readiness predicates (`ObjectStoreUser`, `BucketTopic`,
  `OBCBound`, ...), and the shared timeout tiers.
- `fixture` — create-with-`t.Cleanup` helpers for pure-cleanup resources
  (namespaces, StorageClasses).
- `obc` — ObjectBucketClaim helpers: the provisioner `StorageClass`
  constructor, the create/bound and delete/absent lifecycle waiters, and a
  per-OBC S3 client.
- `secrets` — verification helpers for the Secret references object CRDs
  publish in their status, shared by more than one package.
- `sharedstore` — the shared CephObjectStore fixture.
- `client` — rgw admin, SNS, and S3 client builders and TLS cert generation.

## Anatomy of a package

One exported entry function per package, called from the dispatcher:

```go
func TestObjectStoreUserCaps(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = "test-usercaps"
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: defaultName}}

		osu1 = cephv1.CephObjectStoreUser{ /* fixture literal or constructor */ }

		osuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
	)

	t.Run("ObjectStoreUser caps", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		// ... ordered subtests: act, verify, act, verify ...
	})
}
```

Rules encoded in that shape:

- The entry signature is `(t, k8sh, store)` and never changes. New
  dependencies are added as `Sharedstore` accessors, not parameters.
- The var block declares everything the test uses: fixture objects (use
  constructors — `obc.StorageClass`, package-local helpers like
  `awsKeySecret`/`objectUserKey` — to keep literals one line each), then the
  typed clients bound to locals (`osuClient`, `obcClient`, `obClient`, ...).
  Spelled-out client chains do not appear in test bodies.
- Exactly one outer `t.Run` per package, named `"<Subject> <feature>"`. It
  must be unique across all object packages: it is the `-run` filter handle.
- `ctx := t.Context()` is the first line inside the outer `t.Run` — contexts
  are test-scoped, so nothing outlives its test. (Helpers that wrap their own
  `t.Run` bind their own `ctx := t.Context()` from the subtest's `t`.)
- `fixture.Require*` calls come next. Fixture teardown runs via `t.Cleanup`,
  so it happens even when the test fails partway. Only PURE-CLEANUP teardown
  belongs there; a deletion that asserts behavior ("deleting the OBC must not
  delete the user") is an ordered subtest.
- Repeated multi-assert verification goes in a package-local
  `check<Aspect>(t, deps, subject, expected ...)` helper that wraps its own
  `t.Run` and takes the expected state variadically. Promote a helper to a
  shared `util/` package named for its subject (e.g. `util/secrets`) only
  when a second package needs it.

## Waiting

- State visible to the Kubernetes API uses the watch-based `wait4` verbs:
  `RequireCreate` (create + wait ready), `AssertDelete`/`RequireDelete`
  (delete + wait gone), `RequireCondition`/`AssertCondition` (wait for a
  predicate on an existing resource), `AssertAbsent`/`RequireAbsent` (wait
  for cascade deletion you did not issue). Predicates come from `wait4`
  or are inlined when test-specific.
- State NOT visible to the Kubernetes API (rgw admin ops, S3, SNS) uses
  `wait4.AssertEventually`/`RequireEventually`, the assert/require-flavored
  wrappers over the framework's `utils.Eventually` poll loop. Poll closures
  must not call `assert` or `require` — return an error on transient failures
  (it is surfaced in the timeout message) and assert on captured state after
  the wait (capture-on-success: assign an outer variable inside the closure
  just before returning nil). The closure receives a context bounded by the
  wait deadline; thread it into the reads it performs.
- Log-based state (an operator or endpoint logging a line) uses
  `wait4.AssertPodLog`/`RequirePodLog`, which follows the first matching pod's
  log stream until a line satisfies the predicate or the timeout elapses.
- Timeouts come from the shared tiers: `wait4.TimeoutShort` (routine status
  changes), `TimeoutMedium` (full reconcile, e.g. topic Ready with ARN),
  `TimeoutLong` (first reconcile that may race rgw startup). Bespoke waits
  (e.g. the shared store teardown ladder) pass explicit durations.

## assert vs require

- `require` (and `wait4.Require*`) for anything the rest of the (sub)test
  cannot proceed without: creates, fetches of the object about to be
  inspected, decoding, setup waits. Fail fast to avoid cascade noise.
- `assert` (and `wait4.Assert*`) for the properties under test — so every
  violated property in a subtest is reported together — and for teardown
  deletes, so one stuck finalizer does not strand the rest of cleanup.
- `wait4.RequireDelete` only when later steps depend on the deletion having
  completed.
- Never `assert`/`require` inside an `Eventually` poll closure.

## Naming

- Subtest names are lowercase sentences: imperative for actions
  (`"create CephObjectStoreUser %q"`, `"delete obc %q"`), declarative for
  assertions (`"obc %q has bucketOwner %q set"`, `"no secrets in ns %q"`).
  Resource names are always interpolated with `%q`. The `-v` output of a run
  should read as the full scenario.

## Running and filtering

CI runs the suite as:

```
go test -v -timeout 2400s -failfast -run CephObjectSuite github.com/rook/rook/tests/integration
```

To select one package's tests through the suite:

```
go test -tags ceph_preview -run 'TestCephObjectSuite/TestWithoutTLS/ObjectStoreUser_keys' ./tests/integration
```

All build/vet/lint commands need the `ceph_preview` build tag or the module
does not type-check:

```
go build -tags ceph_preview ./tests/...
go vet -tags ceph_preview ./tests/integration/...
gofmt -l tests/integration
```

These are cluster-backed integration tests; the only end-to-end validation is
the CI object suite.

## Adding a new package (checklist)

1. Create `tests/integration/object/<operator-pkg>/<feature>/` per the layout
   rules above.
2. Write the entry func following the anatomy section; pick a globally unique
   outer `t.Run` name.
3. Wire one dispatcher line in `tests/integration/ceph_object_test.go` and,
   if the package creates CephObjectStoreUsers, add its namespace to the
   shared store's `AllowUsersInNamespaces`.
4. Verify with the build/vet/gofmt commands above.

## Conversion playbook (old-style tests)

Per old test: enumerate its behaviors; pick or create target package(s);
express fixtures as var-block constructors; map k8s waits to `wait4` verbs
and non-k8s polls to `Eventually`; write check* helpers for repeated
verification; wire the dispatcher; delete the old code; retire
`tests/framework/clients` methods whose last consumer is gone.

| old test | target package(s) | still needs (build in that PR) |
|---|---|---|
| `testObjectStoreOperations` deletion-blocked-by-dependents | `tests/integration/object/dependents` | private-store fixture, store condition predicates in `wait4` |
| `createCephObjectStore`/`runObjectE2ETestLite`/deletion asserts + zone.json canary | `tests/integration/object/lifecycle` | store create/health/delete helpers, `Sharedstore.Installer()` accessor |
| upgrade-suite object usage | stays in upgrade suite | switch to typed clients + `wait4` |
