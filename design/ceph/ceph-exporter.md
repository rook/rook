# Ceph exporter

### Goals

Use Ceph exporter as the new source of metrics based on performance counters coming from every Ceph daemon.

## Use case

Ceph exporter was created to avoid performance problems at scale obtaining metrics in clusters with high number of daemons (mainly OSDs).
The Prometheus manager module was not enough performant to obtain the performance counters from all the daemons along the whole Ceph cluster, so the solution for avoid this problem in big clusters is to have a new daemon collecting all the performance counters from all the daemons in each host, and expose them as standard prometheus metrics at /metrics endpoint for prometheus to scrape each of those endpoints.

### Why do we need this daemon in Rook

With the Ceph exporter, the source of metrics changes in the Ceph clusters.
There will be two main sources of metrics:
- Prometheus manager module: It is responsible for exposing all metrics other than ceph daemons performance counters.
- Ceph exporter: It is responsible for exposing only ceph daemons performance counters as prometheus metrics.

Ceph exporter will be deployed in Rook when the `monitoring.metricsDisabled: false` setting is applied in the CephCluster CR and the Ceph version later than 17.2.7.

## Prerequisites

- Ceph daemon socket files must have ceph user permission
- A hostPath is available for ceph daemon socket dirs

## Implementation details

Ceph-exporter is a daemon for collecting perf counters of ceph daemons from admin socket files and exposing them to /metrics endpoint.
So we will need the following things:
- Deploy it similar to crash collector pod
- hostPath for ceph daemon socket dirs under `$dataDirHostPath/exporter`
- Volumes & VolumeMounts for ceph pods & containers for daemon socket files and also ceph config file
- Add daemonSocketDir to ChownCephDataDirsInitContainer()
- Provide a /metrics endpoint: It needs a new K8s service.
- Needs to be scraped by prometheus: We need to use a Prometheus Service Monitor which select the Ceph exporter service

### Exporter Service

The exporter service will be created on each of the node where ceph-exporter pod is running.\
`ceph-exporter-service.yaml`
```yaml
apiVersion: v1
kind: Service
metadata:
  name: ceph-exporter-<node>
  namespace: <namespace>
  labels:
    app: rook-ceph-exporter
    node: <node>
spec:
  internalTrafficPolicy: Cluster
  ports:
  - name: http-metrics
    port: 9926
    protocol: TCP
    targetPort: 9926
  selector:
    app: rook-ceph-exporter
    node: <nodeName>
  sessionAffinity: None
  type: ClusterIP
```

### Exporter ServiceMonitor

A single service monitor that will match all the exporter services.
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: rook-ceph-exporter
  namespace: <namespace>
spec:
  namespaceSelector:
    matchNames:
      - <namespace>
  selector:
    matchLabels:
      app: rook-ceph-exporter
  endpoints:
  - port: http-metrics
    path: /metrics
    interval: 5s
```
