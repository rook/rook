---
title: deploy jaeger tracing components to ceph cluster
target-version: release-1.8
---

# Tracing - Jaeger

## Summary

In order to make it possible to trace in Ceph, we would need the components of jaeger tracing to be deployed in the ceph cluster.

the jaeger components consists of:
1. elasticsearch - needs to be manually deployed.
2. jaeger collector, agent and UI - are all managed by the jaeger Operator.

[Jaeger operator documentation](https://www.jaegertracing.io/docs/1.28/operator/)

### Goals

to be able to see traces from the ceph cluster (rgw and osd) in the jaeger UI.

## Proposal details

When Ceph is deployed via Rook, we would like to add a parameter to the rook CR that will indicate to enable tracing and deploy the jaeger resources.

<b> Ceph cluster yaml file:
```yaml
  apiVersion: ceph.rook.io/v1
  kind: CephCluster
  metadata:
    name: rook-ceph
    namespace: rook-ceph
    [...]
  spec:
    [...]
    tracing:
      enabled: true
    [...]

```
<b> default jaeger CR:
```yaml
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
  name: jaeger-production
spec:
  strategy: production
  collector:
    maxReplicas: 5
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
  storage:
    type: elasticsearch
    options:
      es:
        server-urls: http://elasticsearch:9200
```

This feature won't install the Jaeger operator. it will be required to have ElasticSearch and Jaeger operator pre-installed,
so we will document the process of installing the Jaeger operator, and how to enable tracing in rook CR.

We also will need to add annotations to the rgw and osd deployments in order to auto-inject Jaeger Agent sidecar. 
the annotation as described in Jaeger operator documentation:
```yaml
  annotations:
    "sidecar.jaegertracing.io/inject": "true"
```

