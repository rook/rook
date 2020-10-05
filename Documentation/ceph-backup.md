---
title: Backup
weight: 11250
indent: true
---

# Velero Backup

A backup of Ceph helps you recover your data when a disaster occurs.

## Requirements

The following requirements are needed:

1. Rook Ceph [PVC-based Cluster](ceph-cluster-crd.md#pvc-based-cluster).
2. [Velero](https://velero.io/) + [Restic](https://velero.io/docs/v1.5/restic/)

## Deploy of a Ceph PVC-based Cluster

Firstly, deploy the Ceph operator. After that you need to deploy a Ceph PVC-based Cluster. Please, consider taking a look at the [following documentation](ceph-cluster-crd.md#pvc-based-cluster).

## Install and configure Velero

[Velero](https://velero.io/) is an open source tool to safely backup and restore, perform disaster recovery, and migrate Kubernetes cluster resources and persistent volumes.

For backing up and restoring Kubernetes volumes you need to use the [restic plugin](https://velero.io/docs/v1.5/restic/). You need to add `--use-restic` to velero install command.

Velero supports a lot of cloud providers. Take at look in the [supported providers](https://velero.io/docs/v1.5/supported-providers/) section of the doc for more information and examples. 

For example:
- [GCP](https://github.com/vmware-tanzu/velero-plugin-for-gcp#setup).
- [AWS](https://github.com/vmware-tanzu/velero-plugin-for-aws#setup).
- [Azure](https://github.com/vmware-tanzu/velero-plugin-for-microsoft-azure#setup).


## Create Back Up

We need to save Volumes, Volumes Claims, ConfigMaps, Secrets and Services.

```console
velero backup create <BackupName> --include-namespaces rook-ceph --include-resources pv,pvc,services,configmaps,secrets --default-volumes-to-restic
```

**IMPORTANT:** In addition to backing up the resources in the `rook-ceph` namespace, make sure to back up the bucket's configmaps and secrets that are located in the application namespace.

As Velero change ClusterIP from `MON` services on restore, we need to backup the `yamls` so `MON` can form quorum again.

```console
kubectl get svc -n rook-ceph rook-ceph-mon-a -o yaml > mon-service-a.yaml
kubectl get svc -n rook-ceph rook-ceph-mon-b -o yaml > mon-service-b.yaml
kubectl get svc -n rook-ceph rook-ceph-mon-c -o yaml > mon-service-c.yaml
```

For each of those files we have to delete `metadata.creationTimestamp`, `metadata.ownerReferences`, `metadata.resourceVersion`, `metadata.selfLink` and `metadata.uid`.

For example for file `mon-service-a.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2020-10-04T19:22:31Z"
  labels:
    app: rook-ceph-mon
    ceph_daemon_id: a
    ceph_daemon_type: mon
    mon: a
    mon_cluster: rook-ceph
    pvc_name: rook-ceph-mon-a
    rook_cluster: rook-ceph
  name: rook-ceph-mon-a
  namespace: rook-ceph
  ownerReferences:
  - apiVersion: ceph.rook.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: CephCluster
    name: rook-ceph
    uid: 7561619e-bd35-4649-8af7-a7a0dd7c11b4
  resourceVersion: "3046"
  selfLink: /api/v1/namespaces/rook-ceph/services/rook-ceph-mon-a
  uid: 77e161de-d9ed-41eb-b9c2-386393d21369
spec:
  clusterIP: 10.4.1.210
  ports:
  - name: tcp-msgr1
    port: 6789
    protocol: TCP
    targetPort: 6789
  - name: tcp-msgr2
    port: 3300
    protocol: TCP
    targetPort: 3300
  selector:
    app: rook-ceph-mon
    ceph_daemon_id: a
    mon: a
    mon_cluster: rook-ceph
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
```

We end up with:
```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    app: rook-ceph-mon
    ceph_daemon_id: a
    ceph_daemon_type: mon
    mon: a
    mon_cluster: rook-ceph
    pvc_name: rook-ceph-mon-a
    rook_cluster: rook-ceph
  name: rook-ceph-mon-a
  namespace: rook-ceph
spec:
  clusterIP: 10.4.1.210
  ports:
  - name: tcp-msgr1
    port: 6789
    protocol: TCP
    targetPort: 6789
  - name: tcp-msgr2
    port: 3300
    protocol: TCP
    targetPort: 3300
  selector:
    app: rook-ceph-mon
    ceph_daemon_id: a
    mon: a
    mon_cluster: rook-ceph
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
```

## Restore Back Up

Execute the following command to restore the backup.

```console
velero restore create --from-backup <BackupName>
```

Delete the `MON` services backed up by Velero and restore the old ones.

```console
# Delete MONs
kubectl delete svc -n rook-ceph rook-ceph-mon-a
kubectl delete svc -n rook-ceph rook-ceph-mon-b
kubectl delete svc -n rook-ceph rook-ceph-mon-c

# Restore the new ones
kubectl apply -f mon-service-a.yaml
kubectl apply -f mon-service-b.yaml
kubectl apply -f mon-service-c.yaml
```

Now we need to create cluster with the same settings that were used in the original cluster. In particular, the cluster-on-pvc.yaml settings should be identical or else your cluster may not be restored as expected.

We have to make the same for the `Object Storage`, `Shared Filesystem` and `Block Storage` deployed above Ceph.

### New Cluster

The previous explanation will work with new clusters too. You just need to install velero first and make sure that **service clusterip range is the same as in the previous cluster**.

For example, when creating a GKE cluster from CLI use the flag `--services-ipv4-cidr`. More info [here](https://cloud.google.com/kubernetes-engine/docs/how-to/alias-ips).

In our previous example, `--services-ipv4-cidr=10.4.0.0/16`.