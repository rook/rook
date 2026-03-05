---
title: NVMe-oF Block Storage
---
# NVMe-oF Block Storage

**This feature is experimental**

NVMe over Fabrics (NVMe-oF) allows RBD volumes to be exposed and accessed via the NVMe/TCP protocol. This enables both Kubernetes pods within the cluster and external clients outside the cluster to connect to Ceph block storage using standard NVMe-oF initiators, providing high-performance block storage access over the network.

## Goals

The NVMe-oF integration in Rook serves two primary purposes:

1. **External Client Access**: Rook serves as a backend for external clients outside the cluster, enabling non-Kubernetes workloads to access Ceph block storage through standard NVMe-oF initiators. This allows organizations to leverage their Ceph storage infrastructure for both containerized and traditional workloads.

2. **In-Cluster Consumption**: Pods inside the Kubernetes cluster can consume storage via the NVMe-oF protocol, providing an alternative to traditional RBD mounts with potential performance benefits for certain workloads.

Both use cases are supported, allowing you to choose the appropriate access method based on your specific requirements and deployment scenarios.

For more background and design details, see the [NVMe-oF gateway design doc](https://github.com/rook/rook/blob/master/design/ceph/ceph-nvmeof-gateway.md).
For the Ceph-CSI NVMe-oF design proposal, see the [ceph-csi NVMe-oF proposal](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/nvme-of.md).

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart Guide](../../Getting-Started/quickstart.md).

### Requirements

- **Ceph Version**: Ceph v20 (Tentacle) or later
- **Ceph CSI operator**: v0.6 or later

## Step 1: Create a Ceph Block Pool

Before creating the NVMe-oF gateway, you need to create a CephBlockPool that will be used by the gateway:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: nvmeof
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
```

Create the pool:

```console
kubectl create -f deploy/examples/csi/nvmeof/nvmeof-pool.yaml
```

## Step 2: Create the NVMe-oF Gateway

The `CephNVMeOFGateway` CRD manages the NVMe-oF gateway infrastructure. The operator will automatically create the following resources:

- **Service**: One per gateway instance for service discovery
- **Deployment**: One per gateway instance running the NVMe-oF gateway daemon

Create the gateway:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: nvmeof
  namespace: rook-ceph
spec:
  # Container image for the NVMe-oF gateway daemon
  image: quay.io/ceph/nvmeof:1.5
  # Pool name that will be used by the NVMe-oF gateway
  pool: nvmeof
  # ANA (Asymmetric Namespace Access) group name
  group: group-a
  # Number of gateway instances to run
  instances: 1
  hostNetwork: false
```

Apply the gateway configuration:

```console
kubectl create -f deploy/examples/nvmeof-test.yaml
```

Verify the gateway is running:

```console
kubectl get pod -n rook-ceph -l app=rook-ceph-nvmeof
```

**Example Output**

```console
NAME                                         READY   STATUS    RESTARTS   AGE
rook-ceph-nvmeof-nvmeof-a-85844ff6b8-4r8gj   1/1     Running   0          91s
```

## Step 3: Deploy the NVMe-oF CSI Driver via CSI Operator

The NVMe-oF CSI driver is deployed via the ceph-csi operator.

Apply the `Driver` CR for NVMe-oF that will trigger the creation of the
Ceph-CSI/NVMe-oF deployment and daemonset:

```console
kubectl create -f deploy/examples/csi/nvmeof/driver.yaml
```

Verify the CSI operator created the controller and node plugins:

```console
kubectl get pods -n rook-ceph | grep nvmeof
```

**Example Output**

```console
rook-ceph.nvmeof.csi.ceph.com-ctrlplugin-d9d77fb7c-kkl28   5/5     Running   0          60s
rook-ceph.nvmeof.csi.ceph.com-nodeplugin-xvt5g              2/2     Running   0          60s
```

## Step 4: Create the StorageClass

Create a StorageClass that uses the NVMe-oF CSI driver.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof
parameters:
  clusterID: rook-ceph
  pool: nvmeof
  subsystemNQN: nqn.2016-06.io.spdk:cnode1.rook-ceph
  nvmeofGatewayAddress: "rook-ceph-nvmeof-nvmeof-a.rook-ceph.svc.cluster.local"
  nvmeofGatewayPort: "5500"
  listeners: |
    [
      {
        "hostname": "rook-ceph-nvmeof-nvmeof-a"
      }
    ]
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-modify-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/controller-modify-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-expand-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-expand-secret-namespace: rook-ceph
  imageFormat: "2"
  imageFeatures: layering,deep-flatten,exclusive-lock,object-map,fast-diff
provisioner: rook-ceph.nvmeof.csi.ceph.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
```

!!! note
    The provisioner name `rook-ceph.nvmeof.csi.ceph.com` is prefixed
    with the operator namespace.

```console
kubectl create -f deploy/examples/csi/nvmeof/storageclass.yaml
```

## Step 5: Create a PersistentVolumeClaim

Create a PVC using the NVMe-oF storage class:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nvmeof-external-volume
  namespace: default
spec:
  storageClassName: ceph-nvmeof
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 128Mi
```

!!! note
    This PVC is created for CSI driver provisioning. The volume will be accessible via NVMe-oF protocol by both Kubernetes pods within the cluster and external clients outside the cluster using standard NVMe-oF initiators.

Create the PVC:

```console
kubectl create -f deploy/examples/csi/nvmeof/pvc.yaml
```

Verify the PVC is bound:

```console
kubectl get pvc nvmeof-external-volume
```

**Example Output**

```console
NAME                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
nvmeof-external-volume   Bound    pvc-b4108580-5cfa-46d3-beff-320088a5bf3c   128Mi      RWO            ceph-nvmeof    20m
```

## Step 6: Create a Pod

Create a pod that consumes the NVMe-oF volume:

```console
kubectl create -f deploy/examples/csi/nvmeof/pod.yaml
```

Verify the pod is running:

```console
kubectl get pods -n default nvmeof-test-pod
```

**Example Output**

```console
NAME              READY   STATUS    RESTARTS   AGE
nvmeof-test-pod   1/1     Running   0          60s
```

## Step 7: Accessing Volumes via NVMe-oF

Once the PVC is created and bound, the volume is available via
NVMe-oF. The volume can be accessed by both Kubernetes pods within
the cluster and external clients outside the cluster.

### Access from External Clients

External clients outside the Kubernetes cluster can connect to the gateway using standard NVMe-oF procedures.

#### Prerequisites for External Clients

- **NVMe-oF Initiator**: The client must have the `nvme-tcp` kernel module loaded and `nvme-cli` installed
- **Network Access**: The client must be able to reach the gateway service IP and ports

#### Discover Subsystems

From the external client, discover available NVMe-oF subsystems:

```bash
nvme discover -t tcp -a <gateway-service-ip> -s 5500
```

Replace `<gateway-service-ip>` with the gateway service ClusterIP or an accessible endpoint.

#### Connect to Subsystem

Connect to the discovered subsystem:

```bash
nvme connect -t tcp -n <subsystem-nqn> -a <gateway-ip> -s 5500
```

Replace:

- `<subsystem-nqn>` with the `subsystemNQN` value from your StorageClass (e.g., `nqn.2016-06.io.spdk:cnode1.rook-ceph`)
- `<gateway-ip>` with the gateway service IP or pod IP

#### Access the Volume

Once connected, the NVMe namespace will appear as a block device on the client:

```bash
lsblk | grep nvme
```

The device will typically appear as `/dev/nvmeXnY` where X is the controller number and Y is the namespace ID.

#### Format and Mount (Optional)

If you want to format and mount the device:

```bash
# Format the device
sudo mkfs.ext4 /dev/nvmeXnY

# Mount the device
sudo mkdir /mnt/nvmeof
sudo mount /dev/nvmeXnY /mnt/nvmeof
```

## High Availability

For production deployments, configure multiple gateway instances for high availability:

1. **Increase Gateway Instances**: Set `instances: 2` or higher in the `CephNVMeOFGateway` spec
2. **Update StorageClass Listeners**: Add all gateway deployment hostnames to the `listeners` array
3. **Load Balancing**: Each gateway instance has its own Service; list all of them to support multipath/HA

Example with multiple instances:

```yaml
spec:
  instances: 2
  # ... other settings
```

Then update the StorageClass `listeners` to include all gateway
hostnames:

```yaml
listeners: |
  [
    {
      "hostname": "rook-ceph-nvmeof-nvmeof-a"
    },
    {
      "hostname": "rook-ceph-nvmeof-nvmeof-b"
    }
  ]
```

## Troubleshooting

### Check Gateway Pod Logs

```console
kubectl logs -n rook-ceph -l app=rook-ceph-nvmeof --tail=100
```

### Check CSI Controller Plugin Logs

```console
kubectl logs -n rook-ceph deploy/rook-ceph.nvmeof.csi.ceph.com-ctrlplugin --tail=100
```

### Verify Gateway Service

```console
kubectl describe service -n rook-ceph rook-ceph-nvmeof-nvmeof-a
```

### Check PVC Events

```console
kubectl describe pvc nvmeof-external-volume
```

### Verify Ceph CSI Config

Ensure the `rook-ceph-csi-config` ConfigMap exists and contains the cluster configuration:

```console
kubectl get configmap -n rook-ceph rook-ceph-csi-config -o yaml
```

## Teardown

!!! warning
    Deleting the PVC will also delete the underlying RBD image and NVMe namespace. Ensure you have backups if needed.

To clean up all the artifacts created:

```console
# Delete the test pod
kubectl delete -f deploy/examples/csi/nvmeof/pod.yaml

# Delete the PVC
kubectl delete pvc nvmeof-external-volume

# Delete the StorageClass
kubectl delete storageclass ceph-nvmeof

# Delete the NVMe-oF CSI operator resources
kubectl delete -f deploy/examples/csi/nvmeof/csi-operator-nvmeof.yaml

# Delete the NVMe-oF gateway
kubectl delete -f deploy/examples/nvmeof-test.yaml

# Delete the block pool (optional)
kubectl delete -f deploy/examples/csi/nvmeof/nvmeof-pool.yaml
```

## References

- [Ceph NVMe-oF Documentation](https://docs.ceph.com/en/latest/rbd/nvmeof-overview/)
- [Ceph NVMe-oF Initiator Guide](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux/)
- [Ceph CSI NVMe-oF Support](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/nvme-of.md)
