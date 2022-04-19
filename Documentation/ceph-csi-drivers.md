---
title: Ceph CSI
weight: 3200
indent: true
---
{% include_relative branch.liquid %}
# Ceph CSI Drivers

There are three CSI drivers integrated with Rook that will enable different scenarios:

* RBD: This block storage driver is optimized for RWO pod access where only one pod may access the
  storage. [More information](ceph-block.md).
* CephFS: This file storage driver allows for RWX with one or more pods accessing the same storage.
  [More information](ceph-filesystem.md).
* NFS (experimental): This file storage driver allows creating NFS exports that can be mounted to
  pods, or the exports can be mounted directly via an NFS client from inside or outside the
  Kubernetes cluster. [More information](ceph-nfs-crd.md#ceph-csi-nfs-provisioner-and-nfs-csi-driver).

The Ceph Filesysetem (CephFS) and RADOS Block Device (RBD) drivers are enabled automatically with
the Rook operator. The NFS driver is disabled by default. All drivers will be started in the same
namespace as the operator when the first CephCluster CR is created.

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

### Prerequisites

Kubernetes version 1.21 or greater is required.

### Enable volume replication

1. Install the volume replication CRDs:

```console
kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/v0.3.0/config/crd/bases/replication.storage.openshift.io_volumereplications.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/volume-replication-operator/v0.3.0/config/crd/bases/replication.storage.openshift.io_volumereplicationclasses.yaml
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

## CSI-Addons Controller

The CSI-Addons Controller handles the requests from users to initiate an operation. Users create a CR that the controller inspects, and forwards a request to one or more CSI-Addons side-cars for execution.

### Deploying the controller

Users can deploy the controller by running the following commands:

```bash
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.3.0/deploy/controller/crds.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.3.0/deploy/controller/rbac.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.3.0/deploy/controller/setup-controller.yaml
```

This creates the required crds and configure permissions.

### Enable the CSI-Addons Sidecar

To use the features provided by the CSI-Addons, the `csi-addons`
containers need to be deployed in the RBD provisioner and nodeplugin pods,
which are not enabled by default.

Execute the following command in the cluster to enable the CSI-Addons
sidecar:

* Update the `rook-ceph-operator-config` configmap and patch the
 following configurations

```bash
kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_CSIADDONS": "true"'
```

* After enabling `CSI_ENABLE_CSIADDONS` in the configmap, a new sidecar container with name `csi-addons`
 should now start automatically in the RBD CSI provisioner and nodeplugin pods.

> NOTE: Make sure the version of ceph-csi used is v3.5.0+

### CSI-ADDONS Operation

CSI-Addons supports the following operations:

- Reclaim Space
  - [Creating a ReclaimSpaceJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.3.0/docs/reclaimspace.md#reclaimspacejob)
  - [Creating a ReclaimSpaceCronJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.3.0/docs/reclaimspace.md#reclaimspacecronjob)
  - [Annotating PersistentVolumeClaims](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.3.0/docs/reclaimspace.md#annotating-perstentvolumeclaims)
- Network Fencing
  - [Creating a NetworkFence](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.3.0/docs/networkfence.md)

## Enable RBD Encryption Support

Ceph-CSI supports encrypting individual RBD PersistentVolumeClaim with LUKS encryption. More details can be found
[here](https://github.com/ceph/ceph-csi/blob/v3.6.0/docs/deploy-rbd.md#encryption-for-rbd-volumes)
with full list of supported encryption configurations. A sample configmap can be found
[here](https://github.com/ceph/ceph-csi/blob/v3.6.0/examples/kms/vault/kms-config.yaml).

> NOTE: Rook also supports OSD encryption (see `encryptedDevice` option [here](ceph-cluster-crd.md#osd-configuration-settings)).
Using both RBD PVC encryption and OSD encryption together will lead to double encryption and may reduce read/write performance.

Unlike OSD encryption, existing ceph clusters can also enable Ceph-CSI RBD PVC encryption support and multiple kinds of encryption
KMS can be used on the same ceph cluster using different storageclasses.

Following steps demonstrate how to enable support for encryption:

* Create the `rook-ceph-csi-kms-config` configmap with required encryption configuration in
the same namespace where the Rook operator is deployed. An example is shown below:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-ceph-csi-kms-config
  namespace: rook-ceph
data:
  config.json: |-
    {
      "user-secret-metadata": {
        "encryptionKMSType": "metadata",
        "secretName": "storage-encryption-secret"
      }
    }
```

* Update the `rook-ceph-operator-config` configmap and patch the
 following configurations

```bash
kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_ENCRYPTION": "true"'
```

* Create necessary resources (secrets, configmaps etc) as required by the encryption type.
In this case, create `storage-encryption-secret` secret in the namespace of pvc as shown:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-encryption-secret
  namespace: rook-ceph
stringData:
  encryptionPassphrase: test-encryption
```

* Create a new [storageclass](../deploy/examples/csi/rbd/storageclass.yaml) with additional parameters
`encrypted: "true"` and `encryptionKMSID: "<key used in configmap>"`. An example is show below:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block-encrypted
parameters:
  # additional parameters required for encryption
  encrypted: "true"
  encryptionKMSID: "user-secret-metadata"
# ...
```

* PVCs created using the new storageclass will be encrypted.