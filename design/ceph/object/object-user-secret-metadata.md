# Custom Labels and Annotations on CephObjectStoreUser Secrets

## Summary

A CephObjectStoreUser (COSU) produces a Secret named
`rook-ceph-object-user-<store>-<user>` that holds the user's S3 credentials and endpoint. The
operator sets a fixed set of labels on it (`app`, `user`, `rook_cluster`, `rook_object_store`) and
no annotations. This proposal adds an optional `spec.secretTemplate` to the COSU so that users can
attach their own labels and annotations to that Secret.

## Motivation

Applications often need a COSU's credentials in namespaces other than the one the COSU lives in.
Clusters commonly use a replication tool to copy Secrets between namespaces, and these tools decide
which Secrets to copy from labels and annotations on the source Secret:

* Reflector and kubernetes-replicator copy a Secret when it carries their annotations.
* Kyverno generate policies can clone a Secret into other namespaces, typically matching the
    source Secret by label.

Users cannot add this metadata today. The operator rewrites the whole Secret with an `Update` on
every reconcile, so anything added by hand or by another controller is removed the next time the
COSU reconciles.

Rook already supports this pattern for its own cluster Secrets: the CephCluster
`annotations.clusterMetadata` key adds annotations to the mon and admin keyring Secrets so that they
can be replicated or backed up with kubed. This proposal gives COSU Secrets the equivalent.

## Goals

* Let users declare extra labels and annotations for the Secret that the COSU generates.
* Keep the labels and annotations that Rook sets or acts on under Rook's control.
* Make the CR the only source of truth: removing an entry from the CR removes it from the Secret.
* Reject invalid metadata when the CR is admitted, before any change is made in RGW.

## Non-Goals

* Secrets supplied by the user through `spec.keys`. Rook reads those and never writes them.
* Preserving labels or annotations that other actors write directly to the Secret. The operator
    keeps its current behavior of owning the Secret's metadata. Changing to server-side apply with
    field ownership would be a separate change affecting every Secret the operator writes.
* Other Secrets that Rook generates, such as the CephObjectStoreAccount root-user Secret. The same
    field shape can be added to them later.
* Renaming the Secret or changing its data keys.

## API Changes

A new optional field is added to `ObjectStoreUserSpec`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: "my display name"
  # [Optional] Metadata to add to the Secret that holds this user's credentials.
  secretTemplate:
    labels:
      team: payments
    annotations:
      reflector.v1.k8s.emberstack.com/reflection-allowed: "true"
      reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces: "payments-app"
```

```go
// ObjectUserSecretTemplate defines metadata to add to the Secret that holds a
// CephObjectStoreUser's credentials.
type ObjectUserSecretTemplate struct {
    // Labels to add to the Secret. Keys that Rook sets or acts on are not allowed.
    // +optional
    Labels Labels `json:"labels,omitempty"`
    // Annotations to add to the Secret. Keys that Rook acts on are not allowed.
    // +optional
    Annotations Annotations `json:"annotations,omitempty"`
}
```

The field reuses Rook's existing `Labels` and `Annotations` types. The name and shape follow
cert-manager's `Certificate.spec.secretTemplate`, which solves the same problem for the Secrets that
cert-manager generates. Like cert-manager, template entries cannot override the metadata that the
controller sets itself.

### Reserved Keys

These keys are reserved because Rook sets them or changes its behavior when they are present:

* Labels `app`, `user`, `rook_cluster`, and `rook_object_store`, which Rook sets on the Secret.
    Rook's tests and external consumers select the Secret with them.
* The label `do_not_reconcile`, which stops the operator from repairing changes to the Secret's
    data.
* The annotation `cephx-keyring`, which has the same effect.
* Any label or annotation key whose prefix is `rook.io` or ends in `.rook.io`. Rook acts on
    annotations in this namespace. For example, the CSI controllers choose their credential Secrets
    by `csi.rook.io/*` annotations on Secrets in the cluster namespace. A COSU Secret carrying one
    would break CSI for RadosNamespaces and SubVolumeGroups.

Any label or annotation that Rook adds to this Secret in future must use a `rook.io` prefix, so it
is already reserved and never makes a stored CR invalid.

### Validation

A CEL rule rejects the COSU at admission if either map contains a reserved key, a label key or
annotation key that is not a valid qualified name, or a label value that is not a valid label
value. The syntax checks use the Kubernetes CEL format library (`format.qualifiedName()` and
`format.labelValue()`), which is available from Kubernetes 1.32, Rook's minimum supported version.
Rook already ships a per-key CEL rule on a map (`muteHealthWarning` in the CephCluster health check
settings), so this follows existing precedent.

`MaxProperties` limits both maps. This keeps the CEL rule within the cost budget and keeps the
Secret well below the object size limit. The chosen limit must be shown to fit the budget on a
Kubernetes 1.32 API server.

The operator checks the same rules with the Kubernetes validation helpers before it makes any RGW
change, as Rook already does for user-supplied node labels. This covers CRs stored before the CEL
rule existed. A COSU that fails the check reports `ReconcileFailed` and makes no change to the RGW
user, so the RGW user and its credential Secret cannot drift apart. This matters because the
generated Secret is written after the RGW user is updated: if an invalid label were caught only by
the Secret write, a key rotated through `spec.keys` would already be revoked in RGW while the Secret
still held it. The COSU status has no message field, so the error text appears in a Warning event
on the COSU and in the operator log.

## Reconcile Behavior

* The operator builds the Secret's labels from its own fixed set first and then adds the user's
    labels and annotations without overwriting existing keys.
* The operator writes the full label and annotation set on every reconcile, as it does today.
    An entry removed from `secretTemplate` is removed from the Secret on the next reconcile.
* Changing `secretTemplate` changes the COSU spec, which triggers a reconcile.
* A change to only the Secret's labels or annotations does not trigger a reconcile, because the
    Secret watch predicate discards metadata before deciding. Such changes are overwritten at the
    next reconcile, as they are today. A replication tool that writes back to the source Secret
    therefore cannot cause a reconcile loop.

## Security Considerations

Anyone who can set labels or annotations on a credential Secret can hand it to any controller that
acts on that metadata. For example, a Reflector annotation copies the Secret into another namespace.
After this change, write access to CephObjectStoreUser resources therefore includes that ability.
Default RBAC does not aggregate COSU access into the built-in `edit` or `admin` roles, and those
roles can already write Secrets. The new ability therefore matters only where an administrator has
granted COSU write access to someone who cannot write Secrets in that namespace. The user guide will
state this next to the new field.

Rook reserves the keys that it owns or acts on (see Reserved Keys). It does not try to restrict keys
that belong to third-party tools, because such a list would always be incomplete. Clusters that need
that restriction can apply an admission policy, such as a ValidatingAdmissionPolicy, to the COSU
field.

## Alternatives Considered

### Write the Secret into a different namespace

A `secretNamespace` field would let the COSU write its Secret directly into an application
namespace. It is rejected for these reasons:

* The operator can write Secrets in every namespace, and it overwrites an existing Secret of the
    same name. Today that is safe only because the Secret's name and namespace are fixed to the
    COSU's own. A destination namespace would let a COSU author overwrite any Secret with that name
    in any namespace.
* Owner references cannot cross namespaces, so deleting the Secret would need a finalizer.
* It would need its own namespace allowlist.
* It covers only one destination namespace.

`CephObjectStore.spec.allowUsersInNamespaces` already lets a COSU live in an application namespace,
which puts its Secret there. That remains the right choice when one namespace needs the credentials
and its owners may manage the COSU. This proposal covers credentials that several namespaces need,
and COSUs that administrators keep in the cluster namespace.

### Extend CephCluster `annotations.clusterMetadata`

Applying `clusterMetadata` annotations to every COSU Secret would reuse an existing setting. It is
rejected because it applies the same annotations to every COSU Secret in the cluster, and replication
targets differ from user to user. It also cannot carry labels, and COSUs in other namespaces are not
controlled by the CephCluster author. A cluster-wide default could be added later. It would sit
below `secretTemplate`: Rook's own keys override `secretTemplate`, and `secretTemplate` overrides the
default.

## Upgrade and Downgrade

* **Upgrade:** the field is optional and has no default, so existing COSUs and their Secrets are
    unchanged.
* **Downgrade:** an older operator does not know the field and rebuilds the Secret with only its
    fixed labels, so the custom metadata is removed at its next reconcile. Once the older CRD is
    applied, the API server drops the field from every COSU it returns. The stored data loses it at
    the next write, and until then reinstalling the newer CRD brings it back. Consumers that depend
    on the metadata stop matching after a downgrade.

## Testing

* Unit tests for Secret generation: custom labels and annotations are applied, reserved keys keep
    Rook's values, and a removed entry does not reappear.
* A unit test showing that invalid or reserved metadata fails the operator's check before any RGW
    admin call is made.
* A CEL test against a Kubernetes 1.32 API server (a disposable kind cluster with the CRD applied
    and server-side dry-run). It shows that reserved keys and invalid syntax are rejected, and that
    the rule fits the cost budget at `MaxProperties`.
* An assertion in the existing object-user integration test that a labeled COSU's Secret carries
    the label and can be found with a label selector.
