---
title: mgr-prometheus-metrics-tls
---

## Motivation

Some environment require encryption in transit on intra-cluster traffic as a
compliance rule.

This feature adds HTTPS support to the prometheus server to satisfy that requirement.

## Summary

Secure the Ceph MGR Prometheus metrics endpoint (`rook-ceph-mgr`) with HTTPS.
Ceph supports this natively when the prometheus module TLS settings are
configured ([ceph/ceph#70989](https://github.com/ceph/ceph/pull/70989)).

Rook mounts the TLS Secret, configures Ceph, and updates the Service and
ServiceMonitor so scrapes use HTTPS. Rook does **not** create or renew
certificates.

The server keypair (`tls.crt`, `tls.key`) and the CA Prometheus uses to verify
that cert are separate. The mgr pod only needs the keypair. The ServiceMonitor
may reference a CA from a different Secret or ConfigMap when the cert is not
signed by a public authority.

```
Scraper --HTTPS--> Service :9283 --> mgr metrics :9284 (HTTPS)
                                              │
                                              ▼
                         TLS Secret (tls.crt + tls.key mounted in mgr pod)

Scraper tlsConfig.ca <-- optional Secret or ConfigMap
```

## What Rook does

When `monitoring.metricsTLS` is set and the cluster runs Ceph with prometheus
module TLS support (requires Ceph v20.2.z+, see [ceph/ceph#70989](https://github.com/ceph/ceph/pull/70989)), Rook:

1. **Mounts the TLS Secret** into the mgr container at
   `/etc/ceph/metrics-tls/` (`tls.crt`, `tls.key`).
2. **Configures Ceph** via the mon config store:
   - `mgr/prometheus/ssl = true`
   - `mgr/prometheus/crt_file` / `mgr/prometheus/key_file` → mount paths
   - `mgr/prometheus/ssl_server_port = 9284`
3. **Updates Service `rook-ceph-mgr`**
   - Port name `https-metrics`, Service port `9283` → `targetPort` `9284`
   - Updates **only** `Spec.Ports` on reconcile so existing Service
     annotations are preserved.
4. **Updates ServiceMonitor `rook-ceph-mgr`**
   - Port `https-metrics`, `scheme: https`
   - `tlsConfig.serverName: rook-ceph-mgr.<namespace>.svc`
   - `tlsConfig.ca` from `metricsTLS.ca` when set (Secret or ConfigMap key)
   - When `ca` is omitted, no custom CA is set on the ServiceMonitor
   - Preserves an existing ConfigMap-based CA when reconciling updates
5. **Watches the TLS Secret** and reconciles on change.
6. **Rolls mgr pods on cert rotation** by annotating the Deployment pod
   template with the Secret `resourceVersion` when the `resourceVersion`
   changes.

When `metricsTLS` is removed from the spec, Rook disables the Ceph TLS
settings (`ssl = false`, clear cert paths and ssl port) and reverts the
Service and ServiceMonitor to the default HTTP configuration.

When `metricsTLS` is set on a Ceph version without prometheus TLS support,
Rook fails the reconcile so the cluster is not left with an unintended insecure
endpoint.

## API

```go
// MonitoringSpec
MetricsTLS *MetricsTLSSpec `json:"metricsTLS,omitzero"`

type MetricsTLSSpec struct {
    // SecretName is the Kubernetes Secret containing tls.crt and tls.key for the
    SecretName string `json:"secretName"`

    // CA is an optional trust for the ServiceMonitor tlsConfig.ca.
    // Omit when the server cert chains to a public CA.
    // +optional
    CA *MetricsTLSCASpec `json:"ca,omitempty"`
}

// MetricsTLSCASpec selects a CA bundle from a Secret or ConfigMap.
// Exactly one of secret or configMap must be set.
type MetricsTLSCASpec struct {
    Secret    *corev1.SecretKeySelector    `json:"secret,omitempty"`
    ConfigMap *corev1.ConfigMapKeySelector `json:"configMap,omitempty"`
}
```

The presence of `metricsTLS` enables TLS. `secretName` is required.

Example with a private CA in a ConfigMap:

```yaml
spec:
  monitoring:
    enabled: true
    metricsTLS:
      secretName: rook-ceph-prometheus-server-tls
      ca:
        configMap:
          name: rook-ceph-mgr-metrics-service-ca
          key: service-ca.crt
```

Example with a public CA (no `ca` field):

```yaml
spec:
  monitoring:
    enabled: true
    metricsTLS:
      secretName: rook-ceph-prometheus-server-tls
```

The server Secret must contain `tls.crt` and `tls.key`. `ca.crt` is not
required in that Secret.

## Out of scope

- Creating or renewing TLS certificates
