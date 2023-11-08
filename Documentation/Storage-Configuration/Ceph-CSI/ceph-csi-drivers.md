---
title: Ceph CSI Drivers
---

There are three CSI drivers integrated with Rook that will enable different scenarios:

* RBD: This block storage driver is optimized for RWO pod access where only one pod may access the
  storage. [More information](../Block-Storage-RBD/block-storage.md).
* CephFS: This file storage driver allows for RWX with one or more pods accessing the same storage.
  [More information](../Shared-Filesystem-CephFS/filesystem-storage.md).
* NFS (experimental): This file storage driver allows creating NFS exports that can be mounted to
  pods, or the exports can be mounted directly via an NFS client from inside or outside the
  Kubernetes cluster. [More information](../NFS/nfs-csi-driver.md)

The Ceph Filesystem (CephFS) and RADOS Block Device (RBD) drivers are enabled automatically with
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
$ curl -X GET http://10.109.65.142:9080/metrics 2>/dev/null | grep csi
# HELP csi_liveness Liveness Probe
# TYPE csi_liveness gauge
csi_liveness 1
```

Check the [monitoring doc](../Monitoring/ceph-monitoring.md) to see how to integrate CSI
liveness metrics into ceph monitoring.

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

To support RBD Mirroring, the [CSI-Addons sidecar](https://github.com/csi-addons/kubernetes-csi-addons#readme) will be started in the RBD provisioner pod.
The CSI-Addons supports the VolumeReplication operation. The volume replication controller provides common and reusable APIs for storage disaster recovery. It is based on [csi-addons/spec](https://github.com/csi-addons/spec) specification and can be used by any storage provider.
It follows the controller pattern and provides extended APIs for storage disaster recovery. The extended APIs are provided via Custom Resource Definitions (CRDs).

### Prerequisites

Kubernetes version 1.21 or greater is required.

### Enable CSIAddons Sidecar

To enable the CSIAddons sidecar and deploy the controller, Please follow the steps [below](#csi-addons-controller)

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
Also, See the example manifests for an [RBD ephemeral volume](https://github.com/rook/rook/tree/master/deploy/examples/csi/rbd/pod-ephemeral.yaml) and a [CephFS ephemeral volume](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/pod-ephemeral.yaml).

## CSI-Addons Controller

The CSI-Addons Controller handles the requests from users to initiate an operation. Users create a CR that the controller inspects, and forwards a request to one or more CSI-Addons side-cars for execution.

### Deploying the controller

Users can deploy the controller by running the following commands:

```console
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.7.0/deploy/controller/crds.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.7.0/deploy/controller/rbac.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.7.0/deploy/controller/setup-controller.yaml
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

```console
kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_CSIADDONS": "true"'
```

* After enabling `CSI_ENABLE_CSIADDONS` in the configmap, a new sidecar container with name `csi-addons`
 should now start automatically in the RBD CSI provisioner and nodeplugin pods.

!!! note
    Make sure the version of ceph-csi used is `v3.5.0+`.

### CSI-ADDONS Operation

CSI-Addons supports the following operations:

* Reclaim Space
  * [Creating a ReclaimSpaceJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/reclaimspace.md#reclaimspacejob)
  * [Creating a ReclaimSpaceCronJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/reclaimspace.md#reclaimspacecronjob)
  * [Annotating PersistentVolumeClaims](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/reclaimspace.md#annotating-perstentvolumeclaims)
  * [Annotating Namespace](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/reclaimspace.md#annotating-namespace)
* Network Fencing
  * [Creating a NetworkFence](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/networkfence.md)
* Volume Replication
  * [Creating VolumeReplicationClass](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/volumereplicationclass.md)
  * [Creating VolumeReplication CR](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.7.0/docs/volumereplication.md)

## Enable RBD Encryption Support

Ceph-CSI supports encrypting individual RBD PersistentVolumeClaim with LUKS encryption. More details can be found
[here](https://github.com/ceph/ceph-csi/blob/v3.6.0/docs/deploy-rbd.md#encryption-for-rbd-volumes)
with full list of supported encryption configurations. A sample configmap can be found
[here](https://github.com/ceph/ceph-csi/blob/v3.6.0/examples/kms/vault/kms-config.yaml).

!!! note
    Rook also supports OSD encryption (see `encryptedDevice` option [here](../../CRDs/Cluster/ceph-cluster-crd.md#osd-configuration-settings)).

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

```console
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

* Create a new [storageclass](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/storageclass.yaml) with additional parameters
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

## Enable Read affinity for RBD volumes

Ceph CSI supports mapping RBD volumes with krbd options to allow
serving reads from an OSD in proximity to the client, according to
OSD locations defined in the CRUSH map and topology labels on nodes.

Refer to the [krbd-options](https://docs.ceph.com/en/latest/man/8/rbd/#kernel-rbd-krbd-options)
for more details.

Execute the following steps:

* Patch the `rook-ceph-operator-config` configmap using the following
command.

```console
kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_READ_AFFINITY": "true"'
```

* Add topology labels to the Kubernetes nodes. The same labels may be used as mentioned in the
[OSD topology](../../CRDs/Cluster/ceph-cluster-crd.md#osd-topology) topic.

* (optional) Rook will pass the labels mentioned in [osd-topology](../../CRDs/Cluster/ceph-cluster-crd.md#osd-topology)
as the default set of labels. This can overridden to supply custom labels by updating the
`CSI_CRUSH_LOCATION_LABELS` value in the `rook-ceph-operator-config` configmap.

Ceph CSI will extract the CRUSH location from the topology labels found on the node
and pass it though krbd options during mapping RBD volumes.

!!! note
    This requires kernel version 5.8 or higher.
