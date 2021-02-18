---
title: OpenShift Common Issues
weight: 11400
indent: true
---

# OpenShift Common Issues

## Enable Monitoring in the Storage Dashboard

OpenShift Console uses OpenShift Prometheus for monitoring and populating data in Storage Dashboard. Additional configuration is required to monitor the Ceph Cluster from the storage dashboard.

1. Change the monitoring namespace to `openshift-monitoring`

    Change the namespace of the RoleBinding `rook-ceph-metrics` from `rook-ceph` to `openshift-monitoring` for the `prometheus-k8s` ServiceAccount in [rbac.yaml](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/monitoring/rbac.yaml#L70).

```
subjects:
- kind: ServiceAccount
  name: prometheus-k8s
  namespace: openshift-monitoring
```

2. Enable Ceph Cluster monitoring

    Follow [ceph-monitoring/prometheus-alerts](ceph-monitoring.md#prometheus-alerts).

3. Set the required label on the namespace

    `oc label namespace rook-ceph "openshift.io/cluster-monitoring=true"`

## Troubleshoot Monitoring Issues

> **Pre-req:** Switch to `rook-ceph` namespace with `oc project rook-ceph`

1. Ensure ceph-mgr pod is Running

    ```console
    oc get pods -l app=rook-ceph-mgr
    ```

    >```
    >NAME            READY   STATUS    RESTARTS   AGE
    >rook-ceph-mgr   1/1     Running   0          14h
    >```

2. Ensure service monitor is present

    ```console
    oc get servicemonitor rook-ceph-mgr
    ```

    >```
    >NAME                          AGE
    >rook-ceph-mgr                 14h
    >```

3. Ensure prometheus rules are present

    ```console
    oc get prometheusrules -l prometheus=rook-prometheus
    ```

    >```
    >NAME                    AGE
    >prometheus-ceph-rules   14h
    >```
