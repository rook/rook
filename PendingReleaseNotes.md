# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- `CephObjectStoreUser` gained `spec.secretTemplate`, which adds labels and annotations to the Secret that holds the
  user's credentials, for example to opt the Secret in to a tool that copies Secrets into other namespaces. Keys that
  Rook sets or acts on are reserved.
- RGW SSE-S3 accepts `VAULT_TRANSIT_BACKEND_PATH` in `security.s3.connectionDetails` for a Vault transit engine that is
  not mounted at the default `transit` path. RGW SSE-KMS with the `transit` engine accepts it in
  `security.kms.connectionDetails` too.
- Ceph metrics scraped by Rook's ServiceMonitors now carry a `ceph_cluster` label with the Ceph
  cluster namespace, an unambiguous alias of the existing `cluster` label. A `k8s_cluster` label
  with the name of the Kubernetes cluster is added to all scraped metrics when the new
  `ROOK_K8S_CLUSTER_NAME` operator setting is configured, allowing metrics from multiple
  Kubernetes clusters to be distinguished in centralized monitoring backends.
