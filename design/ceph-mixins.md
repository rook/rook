# Monitoring ceph storage with ceph-mixins

---

## Overview

Currently, rook deploys ceph storage but not ceph monitoring. It can be deployed manually using documented steps. But still, it lacks Prometheus Alerts and Recording Rules that is useful in easy monitoring of ceph storage.

This is where **ceph-mixins** comes in. Ceph-mixins defines a minimalistic approach to package together the ceph storage specific prometheus alerts, recording rules and grafana dashboards, in a simple and platform agnostic way.

Rook can use ceph-mixins as the default solution for monitoring ceph storage.

## Design

---

### Responsibilities

1. Getting the latest  `prometheus-rule.yml` from ceph-mixins into rook.
2. Deploying  `prometheus-rule.yml` .

### Deployment

1. Fetching latest resources from ceph-mixins

   Ceph-mixins will generate a `prometheus-rule.yml` that will be checked-in to rook manually.

    **Changes required in Rook**

    A file will be added to [ceph monitoring](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/ceph/monitoring) directory.

2. Deploying

   1. In Kubernetes

        `kubectl create -f cluster/examples/kubernetes/ceph/monitoring/prometheus-rule.yml`

        This will deploy PrometheusRules (which contains alerts and recording rules).

        _**Note:** It can be deployed in any namespace with a prometheus operator running._

   2. In Openshift

        `oc create -f cluster/examples/kubernetes/ceph/monitoring/prometheus-rule.yml`

         _**Note:** It must be deployed in `openshift-monitoring` namespace which has the prometheus instance._

         If need be, one can deploy own instance of prometheus for monitoring ceph specific resources in desired namespace.

    **Changes required in Rook**

    Documentation needs to be updated.
