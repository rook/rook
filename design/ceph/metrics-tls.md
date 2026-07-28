---
title: mgr-prometheus-metrics-tls
---

## Summary

Secure the Ceph MGR Prometheus metrics endpoint (`rook-ceph-mgr`) with HTTPS.
Ceph supports this natively when the prometheus module TLS settings are
configured ([ceph/ceph#70989](https://github.com/ceph/ceph/pull/70989)).

Rook mounts the TLS Secret, configures Ceph, and updates the Service and
ServiceMonitor so scrapes use HTTPS. Rook does **not** create or renew
certificates.

```
Scraper --HTTPS--> Service :9283 --> mgr metrics :9284 (HTTPS)
                                              │
                                              ▼
                                    TLS Secret (mounted in mgr pod)
```

## What Rook does

When `monitoring.metricsTLS.enabled` is `true` and the cluster runs a Ceph
version with prometheus module TLS support, Rook:

1. **Mounts the TLS Secret** into the mgr container at
   `/etc/ceph/metrics-tls/` (`tls.crt`, `tls.key`).
2. **Configures Ceph** via the mon config store:
   - `mgr/prometheus/ssl = true`
   - `mgr/prometheus/crt_file` / `mgr/prometheus/key_file` → mount paths
   - `mgr/prometheus/ssl_server_port = 9284`
3. **Updates Service `rook-ceph-mgr`**
   - Port name `https-metrics`, Service port `9283` → `targetPort` `9284`
   - Updates **only** `Spec.Ports` on reconcile so existing Service
     annotations (for example OpenShift serving-cert) are
     preserved.
4. **Updates ServiceMonitor `rook-ceph-mgr`**
   - Port `https-metrics`, `scheme: https`
   - `tlsConfig.serverName: rook-ceph-mgr.<namespace>.svc`
   - CA source depends on deployment context (below)
   - Preserves an existing ConfigMap-based CA when reconciling, so
     downstream operators that patch the ServiceMonitor are not overwritten.
5. **Watches the TLS Secret** and reconciles on change.
6. **Rolls mgr pods on cert rotation** by annotating the Deployment pod
   template with the Secret `resourceVersion` when the hash changes.

When `metricsTLS` is present but `enabled` is `false`, Rook disables the Ceph
TLS settings (`ssl = false`, clear cert paths and ssl port).

When TLS is enabled on a Ceph version without #70989 support, Rook does not
apply TLS settings (warn only).

## API

```go
// MonitoringSpec
MetricsTLS *MetricsTLSSpec `json:"metricsTLS,omitzero"`

type MetricsTLSSpec struct {
    Enabled    *bool  `json:"enabled,omitempty"`
    SecretName string `json:"secretName,omitempty"` // default: rook-ceph-prometheus-server-tls
    ManageOpenShiftServiceCAResources bool `json:"manageOpenShiftServiceCAResources,omitempty"`
}
```

Example:

```yaml
spec:
  monitoring:
    enabled: true
    metricsTLS:
      enabled: true
      # secretName: rook-ceph-prometheus-server-tls  # optional
```

Default Secret name: `rook-ceph-prometheus-server-tls`.

## Out of scope

- Creating or renewing TLS certificates
- cert-manager `Certificate` resources
- ocs-operator / `StorageCluster` integration (downstream only)
