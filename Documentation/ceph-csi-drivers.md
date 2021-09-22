---
title: Ceph CSI
weight: 3200
indent: true
---
{% include_relative branch.liquid %}
# Ceph CSI Drivers

There are two CSI drivers integrated with Rook that will enable different scenarios:

* RBD: This driver is optimized for RWO pod access where only one pod may access the storage
* CephFS: This driver allows for RWX with one or more pods accessing the same storage

The drivers are enabled automatically with the Rook operator. They will be started
in the same namespace as the operator when the first CephCluster CR is created.

For documentation on consuming the storage:

* RBD: See the [Block Storage](ceph-block.md) topic
* CephFS: See the [Shared Filesystem](ceph-filesystem.md) topic

## Supported Versions
The supported Ceph CSI version is 3.3.0 or greater with Rook. Refer to ceph csi [releases](https://github.com/ceph/ceph-csi/releases)
for more information.

## Static Provisioning

Both drivers also support the creation of static PV and static PVC from existing RBD image/CephFS volume. Refer to [static PVC](https://github.com/ceph/ceph-csi/blob/devel/docs/static-pvc.md) for more information.

## Configure CSI Drivers in non-default namespace

If you've deployed the Rook operator in a namespace other than "rook-ceph",
change the prefix in the provisioner to match the namespace you used. For
example, if the Rook operator is running in the namespace "my-namespace" the
provisioner value should be "my-namespace.rbd.csi.ceph.com". The same provisioner
name needs to be set in both the storageclass and snapshotclass.

## Liveness Sidecar

All CSI pods are deployed with a sidecar container that provides a prometheus metric for tracking if the CSI plugin is alive and running.
These metrics are meant to be collected by prometheus but can be accesses through a GET request to a specific node ip.
for example `curl -X get http://[pod ip]:[liveness-port][liveness-path]  2>/dev/null | grep csi`
the expected output should be

```console
curl -X GET http://10.109.65.142:9080/metrics 2>/dev/null | grep csi
```

>```
># HELP csi_liveness Liveness Probe
># TYPE csi_liveness gauge
>csi_liveness 1
>```

Check the [monitoring doc](ceph-monitoring.md) to see how to integrate CSI
liveness and grpc metrics into ceph monitoring.

## Dynamically Expand Volume

### Prerequisite

* For filesystem resize to be supported for your Kubernetes cluster, the
  kubernetes version running in your cluster should be >= v1.15 and for block
  volume resize support the Kubernetes version should be >= v1.16. Also,
  `ExpandCSIVolumes` feature gate has to be enabled for the volume resize
  functionality to work.

To expand the PVC the controlling StorageClass must have `allowVolumeExpansion`
set to `true`. `csi.storage.k8s.io/controller-expand-secret-name` and
`csi.storage.k8s.io/controller-expand-secret-namespace` values set in
storageclass. Now expand the PVC by editing the PVC
`pvc.spec.resource.requests.storage` to a higher values than the current size.
Once PVC is expanded on backend and same is reflected size is reflected on
application mountpoint, the status capacity `pvc.status.capacity.storage` of
PVC will be updated to new size.

## RBD Mirroring

To support RBD Mirroring, the [Volume Replication Operator](https://github.com/csi-addons/volume-replication-operator/blob/main/README.md) will be started in the RBD provisioner pod.
The Volume Replication Operator is a kubernetes operator that provides common and reusable APIs for storage disaster recovery. It is based on [csi-addons/spec](https://github.com/csi-addons/spec) specification and can be used by any storage provider.
It follows the controller pattern and provides extended APIs for storage disaster recovery. The extended APIs are provided via Custom Resource Definitions (CRDs).

### Enable volume replication

1. Install the volume replication CRDs:

```console
kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/v0.1.0/config/crd/bases/replication.storage.openshift.io_volumereplications.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/v0.1.0/config/crd/bases/replication.storage.openshift.io_volumereplicationclasses.yaml
```

2. Enable the volume replication controller:
   - For Helm deployments see the [csi.volumeReplication.enabled setting](helm-operator.md#configuration).
   - For non-Helm deployments set `CSI_ENABLE_VOLUME_REPLICATION: "true"` in operator.yaml

## Ephemeral volume support

The generic ephemeral volume feature adds support for specifying PVCs in the
`volumes` field to indicate a user would like to create a Volume as part of the pod spec.
This feature requires the GenericEphemeralVolume feature gate to be enabled.

For example:

```yaml
kind: Pod
apiVersion: v1
...
  volumes:
    - name: mypvc
      ephemeral:
        volumeClaimTemplate:
          spec:
            accessModes: ["ReadWriteOnce"]
            storageClassName: "rook-ceph-block"
            resources:
              requests:
                storage: 1Gi
```

A volume claim template is defined inside the pod spec which refers to a volume
provisioned and used by the pod with its lifecycle. The volumes are provisioned
when pod get spawned and destroyed at time of pod delete.

Refer to [ephemeral-doc](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes) for more info.
Also, See the example manifests for an [RBD ephemeral volume](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples/csi/rbd/pod-ephemeral.yaml) and a [CephFS ephemeral volume](https://github.com/rook/rook/tree/{{ branchName }}/deploy/examples/csi/cephfs/pod-ephemeral.yaml).

### Prerequisites
Kubernetes version 1.21 or greater is required.
