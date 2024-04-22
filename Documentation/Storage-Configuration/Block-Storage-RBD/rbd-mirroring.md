---
title: RBD Mirroring
---

## Disaster Recovery

Disaster recovery (DR) is an organization's ability to react to and recover from an incident that negatively affects business operations.
This plan comprises strategies for minimizing the consequences of a disaster, so an organization can continue to operate – or quickly resume the key operations.
Thus, disaster recovery is one of the aspects of [business continuity](https://en.wikipedia.org/wiki/Business_continuity_planning).
One of the solutions, to achieve the same, is [RBD mirroring](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/).

## RBD Mirroring

[RBD mirroring](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/)
is an asynchronous replication of RBD images between multiple Ceph clusters.
This capability is available in two modes:

* Journal-based: Every write to the RBD image is first recorded
    to the associated journal before modifying the actual image.
    The remote cluster will read from this associated journal and
    replay the updates to its local image.
* Snapshot-based: This mode uses periodically scheduled or
    manually created RBD image mirror-snapshots to replicate
    crash-consistent RBD images between clusters.

!!! note
    This document sheds light on rbd mirroring and how to set it up using rook.
    See also the topic on [Failover and Failback](rbd-async-disaster-recovery-failover-failback.md)

## Create RBD Pools

In this section, we create specific RBD pools that are RBD mirroring
enabled for use with the DR use case.

Execute the following steps on each peer cluster to create mirror enabled pools:

* Create a RBD pool that is enabled for mirroring by adding the section
    `spec.mirroring` in the CephBlockPool CR:

    ```yaml
    apiVersion: ceph.rook.io/v1
    kind: CephBlockPool
    metadata:
    name: mirrored-pool
    namespace: rook-ceph
    spec:
    replicated:
        size: 1
    mirroring:
        enabled: true
        mode: image
    ```

    ```console
    kubectl create -f pool-mirrored.yaml
    ```

* Repeat the steps on the peer cluster.

!!! note
    Pool name across the cluster peers must be the same
    for RBD replication to function.

See the [CephBlockPool documentation](../../CRDs/Block-Storage/ceph-block-pool-crd.md#mirroring) for more details.

!!! note
    It is also feasible to edit existing pools and
    enable them for replication.

## Bootstrap Peers

In order for the rbd-mirror daemon to discover its peer cluster, the
peer must be registered and a user account must be created.

The following steps enable bootstrapping peers to discover and authenticate to each other:

* For Bootstrapping a peer cluster its bootstrap secret is required. To determine the name of the secret that contains the bootstrap secret execute the following command on the remote cluster (cluster-2)

```console
[cluster-2]$ kubectl get cephblockpool.ceph.rook.io/mirrored-pool -n rook-ceph -ojsonpath='{.status.info.rbdMirrorBootstrapPeerSecretName}'
```

Here, `pool-peer-token-mirrored-pool` is the desired bootstrap secret name.

* The secret pool-peer-token-mirrored-pool contains all the information related to the token and needs to be injected to the peer, to fetch the decoded secret:

    ```console
    [cluster-2]$ kubectl get secret -n rook-ceph pool-peer-token-mirrored-pool -o jsonpath='{.data.token}'|base64 -d
    eyJmc2lkIjoiNGQ1YmNiNDAtNDY3YS00OWVkLThjMGEtOWVhOGJkNDY2OTE3IiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFDZ3hmZGdxN013R0JBQWZzcUtCaGpZVjJUZDRxVzJYQm5kemc9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMzkuMzY6MzMwMCx2MToxOTIuMTY4LjM5LjM2OjY3ODldIn0=
    ```

* With this Decoded value, create a secret on the primary site (cluster-1):

    ```console
    [cluster-1]$ kubectl -n rook-ceph create secret generic rbd-primary-site-secret --from-literal=token=eyJmc2lkIjoiNGQ1YmNiNDAtNDY3YS00OWVkLThjMGEtOWVhOGJkNDY2OTE3IiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFDZ3hmZGdxN013R0JBQWZzcUtCaGpZVjJUZDRxVzJYQm5kemc9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMzkuMzY6MzMwMCx2MToxOTIuMTY4LjM5LjM2OjY3ODldIn0= --from-literal=pool=mirrored-pool
    ```

* This completes the bootstrap process for cluster-1 to be peered with cluster-2.
* Repeat the process switching cluster-2 in place of cluster-1, to complete the bootstrap process across both peer clusters.

For more details, refer to the official rbd mirror documentation on
[how to create a bootstrap peer](https://docs.ceph.com/en/latest/rbd/rbd-mirroring/#bootstrap-peers).

## Configure the RBDMirror Daemon

Replication is handled by the rbd-mirror daemon. The rbd-mirror daemon
is responsible for pulling image updates from the remote, peer cluster,
and applying them to image within the local cluster.

Creation of the rbd-mirror daemon(s) is done through the custom resource definitions (CRDs), as follows:

* Create mirror.yaml, to deploy the rbd-mirror daemon

    ```yaml
    apiVersion: ceph.rook.io/v1
    kind: CephRBDMirror
    metadata:
    name: my-rbd-mirror
    namespace: rook-ceph
    spec:
    # the number of rbd-mirror daemons to deploy
    count: 1
    ```

* Create the RBD mirror daemon

    ```console
    [cluster-1]$ kubectl create -f mirror.yaml -n rook-ceph
    ```

* Validate if `rbd-mirror` daemon pod is now up

    ```console
    [cluster-1]$ kubectl get pods -n rook-ceph
    rook-ceph-rbd-mirror-a-6985b47c8c-dpv4k  1/1  Running  0  10s
    ```

* Verify that daemon health is OK

    ```console
    kubectl get cephblockpools.ceph.rook.io mirrored-pool -n rook-ceph -o jsonpath='{.status.mirroringStatus.summary}'
    {"daemon_health":"OK","health":"OK","image_health":"OK","states":{"replaying":1}}
    ```

* Repeat the above steps on the peer cluster.

See the [CephRBDMirror CRD](../../CRDs/Block-Storage/ceph-rbd-mirror-crd.md) for more details on the mirroring settings.

## Add mirroring peer information to RBD pools

Each pool can have its own peer. To add the peer information, patch the already created mirroring enabled pool
to update the CephBlockPool CRD.

```console
[cluster-1]$ kubectl -n rook-ceph patch cephblockpool mirrored-pool --type merge -p '{"spec":{"mirroring":{"peers": {"secretNames": ["rbd-primary-site-secret"]}}}}'
```

## Create VolumeReplication CRDs

Volume Replication Operator follows controller pattern and provides extended
APIs for storage disaster recovery. The extended APIs are provided via Custom
Resource Definition(CRD). Create the VolumeReplication CRDs on all the peer clusters.

```console
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.5.0/config/crd/bases/replication.storage.openshift.io_volumereplicationclasses.yaml
kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.5.0/config/crd/bases/replication.storage.openshift.io_volumereplications.yaml
```

## Enable CSI Replication Sidecars

To achieve RBD Mirroring, `csi-omap-generator` and `csi-addons`
containers need to be deployed in the RBD provisioner pods, which are not enabled by default.

* **Omap Generator**: Omap generator is a sidecar container that when
    deployed with the CSI provisioner pod, generates the internal CSI
    omaps between the PV and the RBD image. This is required as static PVs are
    transferred across peer clusters in the DR use case, and hence
    is needed to preserve PVC to storage mappings.

* **Volume Replication Operator**: Volume Replication Operator is a
    kubernetes operator that provides common and reusable APIs for
    storage disaster recovery. The volume replication operation is
    supported by the [CSIAddons](https://github.com/csi-addons/kubernetes-csi-addons#readme)
    It is based on [csi-addons/spec](https://github.com/csi-addons/spec)
    specification and can be used by any storage provider.

Execute the following steps on each peer cluster to enable the OMap generator and CSIADDONS sidecars:

* Edit the `rook-ceph-operator-config` configmap and add the following configurations

    ```console
    kubectl edit cm rook-ceph-operator-config -n rook-ceph
    ```

    Add the following properties if not present:

    ```yaml
    data:
    CSI_ENABLE_OMAP_GENERATOR: "true"
    CSI_ENABLE_CSIADDONS: "true"
    ```

* After updating the configmap with those settings, two new sidecars
    should now start automatically in the CSI provisioner pod.

* Repeat the steps on the peer cluster.

## Volume Replication Custom Resources

VolumeReplication CRDs provide support for two custom resources:

* **VolumeReplicationClass**: *VolumeReplicationClass* is a cluster scoped
resource that contains driver related configuration parameters. It holds
the storage admin information required for the volume replication operator.

* **VolumeReplication**: *VolumeReplication* is a namespaced resource that contains references to storage object to be replicated and VolumeReplicationClass
corresponding to the driver providing replication.

## Enable mirroring on a PVC

Below guide assumes that we have a PVC (rbd-pvc) in BOUND state; created using
**StorageClass** with `Retain` reclaimPolicy.

```console
[cluster-1]$ kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
rbd-pvc   Bound    pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec   1Gi        RWO            csi-rbd-sc   44s
```

### Create a Volume Replication Class CR

In this case, we create a Volume Replication Class on cluster-1

```console
[cluster-1]$ kubectl apply -f deploy/examples/volume-replication-class.yaml
```

!!! note
    The `schedulingInterval` can be specified in formats of
    minutes, hours or days using suffix `m`, `h` and `d` respectively.
    The optional schedulingStartTime can be specified using the ISO 8601
    time format.

### Create a VolumeReplication CR

* Once VolumeReplicationClass is created, create a Volume Replication for
    the PVC which we intend to replicate to secondary cluster.

```console
[cluster-1]$ kubectl apply -f deploy/examples/volume-replication.yaml
```

!!! note
    :memo: `VolumeReplication` is a namespace scoped object. Thus,
    it should be created in the same namespace as of PVC.

### Checking Replication Status

`replicationState` is the state of the volume being referenced.
Possible values are primary, secondary, and resync.

* `primary` denotes that the volume is primary.
* `secondary` denotes that the volume is secondary.
* `resync` denotes that the volume needs to be resynced.

To check VolumeReplication CR status:

```console
[cluster-1]$kubectl get volumereplication pvc-volumereplication -oyaml
```

```yaml
...
spec:
  dataSource:
    apiGroup: ""
    kind: PersistentVolumeClaim
    name: rbd-pvc
  replicationState: primary
  volumeReplicationClass: rbd-volumereplicationclass
status:
  conditions:
  - lastTransitionTime: "2021-05-04T07:39:00Z"
    message: ""
    observedGeneration: 1
    reason: Promoted
    status: "True"
    type: Completed
  - lastTransitionTime: "2021-05-04T07:39:00Z"
    message: ""
    observedGeneration: 1
    reason: Healthy
    status: "False"
    type: Degraded
  - lastTransitionTime: "2021-05-04T07:39:00Z"
    message: ""
    observedGeneration: 1
    reason: NotResyncing
    status: "False"
    type: Resyncing
  lastCompletionTime: "2021-05-04T07:39:00Z"
  lastStartTime: "2021-05-04T07:38:59Z"
  message: volume is marked primary
  observedGeneration: 1
  state: Primary
```

## Backup & Restore

!!! note
    To effectively resume operations after a failover/relocation,
    backup of the kubernetes artifacts like deployment, PVC, PV, etc need to be created beforehand by the admin; so that the application can be restored on the peer cluster.

Here, we take a backup of PVC and PV object on one site, so that they can be restored later to the peer cluster.

### **Take backup on cluster-1**

* Take backup of the PVC `rbd-pvc`

```console
[cluster-1]$ kubectl  get pvc rbd-pvc -oyaml > pvc-backup.yaml
```

* Take a backup of the PV, corresponding to the PVC

```console
[cluster-1]$ kubectl get pv/pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec -oyaml > pv_backup.yaml
```

!!! note
    We can also take backup using external tools like **Velero**.
    See [velero documentation](https://velero.io/docs/main/) for more information.

#### **Restore the backup on cluster-2**

* Create storageclass on the secondary cluster

```console
[cluster-2]$ kubectl create -f deploy/examples/csi/rbd/storageclass.yaml
```

* Create VolumeReplicationClass on the secondary cluster

```console
[cluster-1]$ kubectl apply -f deploy/examples/volume-replication-class.yaml
volumereplicationclass.replication.storage.openshift.io/rbd-volumereplicationclass created
```

* If Persistent Volumes and Claims are created manually on the secondary cluster,
    remove the `claimRef` on the backed up PV objects in yaml files; so that the
    PV can get bound to the new claim on the secondary cluster.

```yaml
...
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  claimRef:
    apiVersion: v1
    kind: PersistentVolumeClaim
    name: rbd-pvc
    namespace: default
    resourceVersion: "64252"
    uid: 65dc0aac-5e15-4474-90f4-7a3532c621ec
  csi:
...
```

* Apply the Persistent Volume backup from the primary cluster

```console
[cluster-2]$ kubectl create -f pv-backup.yaml
```

* Apply the Persistent Volume claim from the restored backup

```console
[cluster-2]$ kubectl create -f pvc-backup.yaml
```

```console
[cluster-2]$ kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
rbd-pvc   Bound    pvc-65dc0aac-5e15-4474-90f4-7a3532c621ec   1Gi        RWO            rook-ceph-block   44s
```
