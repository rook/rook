---
title: Node Maintenance
---

This guide outlines a comprehensive procedure for safely shutting down, maintaining, and restarting a nodes in the Rook Ceph cluster.

Although updating nodes by failure domain (such as zones or racks) is preferred to minimize downtime, this guide describes the full shutdown process if downtime is acceptable.

## Overview

The basic maintenance procedure involves the following steps:

1. Stop I/O: Scale down all applications to stop any I/O activity to Ceph.
2. Set Ceph Flags: Optionally set the noout (and related) flags to prevent Ceph from marking OSDs as out.
3. Shutdown Ceph Components: Scale down all Ceph deployments (with mons being the last) while ensuring the operator is scaled down first.
4. Perform Maintenance: Conduct the necessary maintenance on your nodes.
5. Restart Ceph Components: Scale up Ceph deployments (starting with mons) and then bring up the operator.
6. Unset Ceph Flags: Remove any flags that were set before the shutdown.
7. Restart Applications: Scale applications back up to resume operations.

## Pre-Maintenance Steps

Backup & Notifications: Always ensure you have a recent backup and inform stakeholders of the planned downtime.

Update Strategy: Whenever possible, plan node updates by failure domain to avoid full cluster downtime.

### Environment

These instructions will work for as long the environment is parameterized correctly.
Set the following environment variables, which will be used throughout this document.

```console
# Parameterize the environment
export ROOK_OPERATOR_NAMESPACE=rook-ceph
export ROOK_CLUSTER_NAMESPACE=rook-ceph
```

## Shutdown Procedure

### **1. Scale Down Applications**

Before any Ceph-specific actions, scale down all applications interacting with Ceph to ensure no new I/O is initiated.

### **2. Set Ceph Flags**

Setting the noout flag (and any additional flags) can help prevent Ceph from marking OSDs as out during the maintenance window.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE exec -it deploy/rook-ceph-tools -- bash
ceph osd set noout
# Optionally, set other flags as needed (e.g., norebalance, nodown)
```

!!! tip
    The ordering of Ceph flags is not critical. You may set them in any order during shutdown and unset them in reverse order during startup.

### **3. Scale Down the Operator**

Scale down the rook-ceph-operator first to avoid it automatically restarting deployments you are scaling down.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE scale deployment rook-ceph-operator --replicas=0
```

### **4. Scale Down Ceph Deployments**

Scale down the Ceph components in a controlled order. For example, you may choose the following order:

```console
RGW deployments > CSI CephFS plugin provisioners > CSI RBD plugin provisioners > OSD deployments > MON deployments > MGR deployments
```

A script to scale these down looks like:

```console
for _category in rook-ceph-rgw csi-cephfsplugin-provisioner csi-rbdplugin-provisioner rook-ceph-osd rook-ceph-mon rook-ceph-mgr rook-ceph-exporter rook-ceph-crashcollector; do
    for _item in $(kubectl get deployment -n rook-ceph | awk '/^'"${_category}"'/{print $1}'); do
        kubectl -n rook-ceph scale deployment ${_item} --replicas=0;
        while [[ $(kubectl get deployment -n rook-ceph ${_item} -o jsonpath='{.status.readyReplicas}') != "" ]]; do
            sleep 5;
        done;
    done;
done
```

### **5. Node Maintenance**

With all Ceph components safely scaled down, you can now perform your node maintenance tasks such as OS upgrades, hardware replacements, or other planned updates.

## Startup Procedure

### **1. Scale Up Ceph Components**

Bring up the Ceph deployments in the following order to help ensure a healthy state before the operator takes over:

Scale up MON deployments first:

```console
for _item in $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE | awk '/^rook-ceph-mon/{print $1}'); do
    kubectl -n $ROOK_CLUSTER_NAMESPACE scale deployment ${_item} --replicas=1;
    while [[ $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE ${_item} -o jsonpath='{.status.replicas}') != "1" ]]; do
        sleep 5;
    done;
done
```

Then scale up OSD and MGR deployments:

```console
for _category in rook-ceph-mgr rook-ceph-osd; do
    for _item in $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE | awk '/^'"${_category}"'/{print $1}'); do
        kubectl -n $ROOK_CLUSTER_NAMESPACE scale deployment ${_item} --replicas=1;
        while [[ $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE ${_item} -o jsonpath='{.status.replicas}') != "1" ]]; do
            sleep 5;
        done;
    done;
done
```

Finally, scale up other deployments:

```console
for _category in rook-ceph-exporter rook-ceph-crashcollector; do
    for _item in $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE | awk '/^'"${_category}"'/{print $1}'); do
        kubectl -n $ROOK_CLUSTER_NAMESPACE scale deployment ${_item} --replicas=1;
        while [[ $(kubectl get deployment -n $ROOK_CLUSTER_NAMESPACE ${_item} -o jsonpath='{.status.replicas}') != "1" ]]; do
            sleep 5;
        done;
    done;
done
```

### **2. Scale Up the Operator**

Once the Ceph components are stable and healthy, scale the operator back up to allow it to reconcile any remaining changes.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE scale deployment rook-ceph-operator --replicas=1
```

!!! tip
    Bringing the operator up after the core components ensures a smoother reconciliation process.

### **3. Unset Ceph Flags**

After the Ceph cluster is back online and healthy, unset any flags that were previously set.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE exec -it deploy/rook-ceph-tools -- bash
ceph osd unset noout
# Unset any additional flags as necessary
```

### **4. Scale Up Applications**

Finally, scale your applications back to their original replica count to resume normal operations.

### CSI Plugins Behavior

Observation: Even when the csi-cephfsplugin-provisioner and csi-rbdplugin-provisioner deployments are scaled to 0 replicas, the corresponding CSI plugin pods (from DaemonSets) remain running.

Explanation: Scaling down the csi-cephfsplugin-provisioner and csi-rbdplugin-provisioner deployments prevents new provisioning requests, but the CSI plugin pods persist because they are managed by DaemonSets. Since DaemonSets are stateless, they can be deleted if needed, and the Rook operator will automatically recreate them when restarted. However, deletion is not required to stop provisioning.
