---
title: OpenShift Common Issues
weight: 10620
indent: true
---

# OpenShift Common Issues

## Enable Monitoring in the Storage Dashboard

OpenShift Console uses OpenShift Prometheus for monitoring and populating data in Storage Dashboard. Additional configuration is required to monitor the Ceph Cluster from the storage dashboard.

1. Enable Ceph Cluster monitoring

    Follow [ceph-monitoring/prometheus-alerts](https://github.com/rook/rook/blob/master/Documentation/ceph-monitoring.md#prometheus-alerts).

2. Set the required label on the namespace

    `$ oc label namespace rook-ceph "openshift.io/cluster-monitoring=true"`

## Troubleshoot Monitoring Issues

> **Pre-req:** Switch to `rook-ceph` namespace with `oc project rook-ceph`

1. Ensure ceph-mgr pod is Running

    `$ oc get pods -l app=rook-ceph-mgr`

    ```bash
    NAME            READY   STATUS    RESTARTS   AGE
    rook-ceph-mgr   1/1     Running   0          14h
    ```

2. Ensure service monitor is present

    `$ oc get servicemonitor rook-ceph-mgr`

    ```bash
    NAME                          AGE
    rook-ceph-mgr                 14h
    ```

3. Ensure prometheus rules are present

    `oc get prometheusrules -l prometheus=rook-prometheus`

    ```bash
    NAME                    AGE
    prometheus-ceph-rules   14h
    ```
