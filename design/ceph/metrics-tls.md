---
title: mgr-prometheus-metrics-tls
---

## Summary

Allow the Ceph MGR Prometheus metrics endpoint (default port 9283) to be
scraped over HTTPS using a Kubernetes TLS Secret.

TLS is terminated by a sidecar in the mgr pod that proxies to the existing
HTTP prometheus module on localhost. On OpenShift, the Secret can be
provisioned automatically via [service-serving
certificates](https://docs.redhat.com/en/documentation/openshift_container_platform/4.22/html/security_and_compliance/configuring-certificates#understanding-service-serving_service-serving-certificate).

### Goals

Optional HTTPS for MGR Prometheus metrics via the CephCluster CR, using a
sidecar for TLS and updating the Service and ServiceMonitor so scrapes work
over HTTPS.

## Proposal details

### Architecture

```
Prometheus  --HTTPS-->  Service :9283  -->  metrics-tls-proxy sidecar :9284
                                              │
                                              │ HTTP localhost
                                              ▼
                                         mgr container :9283
```

| Component | Role |
|---|---|
| mgr container | Unchanged; prometheus module on `:9283` HTTP |
| metrics-tls-proxy sidecar | HTTPS on `:9284`; proxy to `http://127.0.0.1:9283` |
| TLS Secret | Mounted into the sidecar (`tls.crt`, `tls.key`) |
| Service | Port `9283` → `targetPort` `9284` when TLS enabled |
| ServiceMonitor | `scheme: https`, `serverName`, CA trust |

### API

```go
// MonitoringSpec
MetricsTLS *MetricsTLSSpec `json:"metricsTLS,omitempty"`

type MetricsTLSSpec struct {
    Enabled    bool   `json:"enabled,omitempty"`
    SecretName string `json:"secretName,omitempty"` // default: rook-ceph-prometheus-server-tls
}
```

Example:

```yaml
spec:
  monitoring:
    enabled: true
    metricsTLS:
      enabled: true
      # secretName: rook-ceph-prometheus-server-tls
```

### Flow

```
metricsTLS.enabled: true
        │
        ├─► TLS Secret
        │     OpenShift: annotate Service with
        │       service.beta.openshift.io/serving-cert-secret-name
        │       → platform creates Secret (tls.crt / tls.key)
        │     Non-OpenShift: user or cert-manager creates a
        │       kubernetes.io/tls Secret (default name
        │       rook-ceph-prometheus-server-tls, or secretName)
        │
        ├─► Update rook-ceph-mgr Service
        │     port name: https-metrics
        │     targetPort: 9284  (TLS sidecar)
        │
        ├─► CA for Prometheus trust
        │     Prefer ca.crt from the TLS Secret when present
        │     OpenShift fallback: ConfigMap with inject-cabundle
        │       (service-ca.crt)
        │
        ├─► Ensure nginx proxy ConfigMap
        │
        ├─► Mgr Deployment
        │     - mount TLS Secret + nginx ConfigMap
        │     - add metrics-tls-proxy sidecar
        │     - annotate pod template with Secret resourceVersion for rollout
        │
        └─► ServiceMonitor
              port: https-metrics
              scheme: https
              tlsConfig.serverName: rook-ceph-mgr.<ns>.svc
              tlsConfig.ca: Secret ca.crt, else OpenShift service-ca ConfigMap
```

### OpenShift service-serving certificates

When TLS is enabled, Rook annotates the metrics Service with
`service.beta.openshift.io/serving-cert-secret-name`.
OpenShift service-ca creates a Secret with `tls.crt` / `tls.key` only.

Prometheus trust uses a dedicated ConfigMap with
`service.beta.openshift.io/inject-cabundle=true` (`service-ca.crt`).
`tlsConfig.serverName` is set to the Service DNS name.

### Non-OpenShift / cert-manager

Users create a `kubernetes.io/tls` Secret with `tls.crt`, `tls.key`, and
**`ca.crt`**. Prometheus needs that CA to verify the server certificate.
Use cert-manager (or create the Secret manually) so `ca.crt` is present.

### Secret rotation

When the TLS Secret is renewed, nginx does not automatically load the new
certificate. Rook watches the Secret and, on change, updates an annotation on
the mgr Deployment so Kubernetes rolls the pods and the sidecar starts with
the new certs.
