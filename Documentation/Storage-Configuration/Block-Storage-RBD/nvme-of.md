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
- **Disable the Ceph CSI operator**: We are still updating the Ceph CSI operator with NVMe-oF support. Currently, it is required to disable the CSI operator to test NVMe-oF. In operator.yaml, set `ROOK_USE_CSI_OPERATOR: "false"`.

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

## Step 4: Deploy the NVMe-oF CSI Driver

The NVMe-oF CSI driver handles dynamic provisioning of volumes. Deploy the CSI provisioner with the NVMe-oF driver.

Deploy the NVMe-oF CSI provisioner from the example manifest:

```console
kubectl create -f deploy/examples/csi/nvmeof/provisioner.yaml
```

Verify the CSI provisioner pod is running:

```console
kubectl get pods -n rook-ceph -l app=csi-nvmeofplugin-provisioner
```

**Example Output**

```console
NAME                                           READY   STATUS    RESTARTS   AGE
csi-nvmeofplugin-provisioner-65b4fbbc8-jjsqj   4/4     Running   0          75s
```

## Step 5: Create the StorageClass

Create a StorageClass that uses the NVMe-oF CSI driver. You'll need to gather the following information from the gateway:

1. **nvmeofGatewayAddress**: A stable address for the gateway management API
2. **nvmeofGatewayPort**: The gateway port (default: 5500)
3. **listeners**: A JSON array containing listener information for each gateway instance

Discover the values to use in the StorageClass:

1. **nvmeofGatewayAddress**: Use the Service `CLUSTER-IP`.

    ```console
    kubectl get service -n rook-ceph rook-ceph-nvmeof-nvmeof-a
    ```

    **Example Output**

    ```console
    NAME                        TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)                               AGE
    rook-ceph-nvmeof-nvmeof-a   ClusterIP   10.106.98.71   <none>        4420/TCP,5500/TCP,5499/TCP,8009/TCP   24m
    ```

2. **listeners.address**: Use the gateway pod IP.

    ```console
    kubectl get pods -n rook-ceph -l app=rook-ceph-nvmeof -o wide

    ```

    **Example Output**

    ```console
    NAME                                         READY   STATUS    RESTARTS   AGE   IP            NODE       NOMINATED NODE   READINESS GATES
    rook-ceph-nvmeof-nvmeof-a-5fd6cd4d46-mrbwk   1/1     Running   0          26m   10.244.0.16   minikube   <none>           <none>
    ```

3. **listeners.hostname**: Use the gateway deployment name.

    ```console
    kubectl get deployments.apps -n rook-ceph -l app=rook-ceph-nvmeof
    ```

    **Example Output**

    ```console
    NAME                        READY   UP-TO-DATE   AVAILABLE   AGE
    rook-ceph-nvmeof-nvmeof-a   1/1     1            1           27m
    ```

Create the StorageClass:


```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof
parameters:
  clusterID: rook-ceph
  pool: nvmeof
  subsystemNQN: nqn.2016-06.io.spdk:cnode1.rook-ceph
  # Management API - talks to gateway to create subsystems/namespaces
  nvmeofGatewayAddress: "10.106.98.71"
  nvmeofGatewayPort: "5500"
  # Data Plane - worker nodes connect here for actual I/O
  # List ALL gateway pods for HA and multipath
  listeners: |
    [
      {
        "address": "10.244.0.16",
        "port": 4420,
        "hostname": "rook-ceph-nvmeof-nvmeof-a"
      }
    ]
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph
  imageFormat: "2"
  imageFeatures: layering,deep-flatten,exclusive-lock,object-map,fast-diff
provisioner: nvmeof.csi.ceph.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: false
```

Create the StorageClass:

```console
kubectl create -f deploy/examples/csi/nvmeof/storageclass.yaml
```

## Step 6: Create a PersistentVolumeClaim

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

## Step 7: Deploy the NVMe-oF CSI Node Plugin

Deploy the NVMe-oF CSI node plugin:

```console
kubectl create -f deploy/examples/csi/nvmeof/node-plugin.yaml
```

Verify the node plugin pod is running:

```console
kubectl get pods -n rook-ceph -l app=nvmeof.csi.ceph.com-nodeplugin
```

**Example Output**

```console
NAME                               READY   STATUS    RESTARTS   AGE
nvmeof.csi.ceph.com-nodeplugin-xnm82   2/2     Running   0          31h
```

## Step 8: Accessing Volumes via NVMe-oF

Once the PVC is created and bound, the volume is available via NVMe-oF. The volume can be accessed by both Kubernetes pods within the cluster and external clients outside the cluster.

### Access from Kubernetes Pods

Kubernetes pods can consume NVMe-oF volumes by mounting the PVC directly. The CSI driver handles the NVMe-oF connection automatically when the pod mounts the volume.

Create a sample pod that mounts the PVC:

```console
kubectl create -f deploy/examples/csi/nvmeof/pod.yaml
```

Verify the pod is running:

```console
kubectl get pods -n default
```

**Example Output**

```console
NAME              READY   STATUS    RESTARTS   AGE
nvmeof-test-pod   1/1     Running   0          29h
```

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
2. **Update StorageClass Listeners**: Add all gateway instance addresses and instance names to the `listeners` array
3. **Load Balancing**: Each gateway instance has its own Service; list all of them to support multipath/HA

Example with multiple instances:

```yaml
spec:
  instances: 2
  # ... other settings
```

Then update the StorageClass `listeners` to include all gateway instances/services:

```yaml
listeners: |
  [
    {
      "address": "10.99.212.218",
      "port": 4420,
      "hostname": "rook-ceph-nvmeof-nvmeof-a"
    },
    {
      "address": "10.99.212.219",
      "port": 4420,
      "hostname": "rook-ceph-nvmeof-nvmeof-b"
    }
  ]
```

## Troubleshooting

### Check Gateway Pod Logs

```console
kubectl logs -n rook-ceph -l app=rook-ceph-nvmeof --tail=100
```

### Check CSI Provisioner Logs

```console
kubectl logs -n rook-ceph -l app=csi-nvmeofplugin-provisioner --tail=100
```

### Verify Gateway Service

```console
kubectl describe service -n rook-ceph rook-ceph-nvmeof-my-nvmeof-0
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

# Delete the NVMe-oF CSI node plugin
kubectl delete -f deploy/examples/csi/nvmeof/node-plugin.yaml

# Delete the NVMe-oF CSI provisioner (includes ServiceAccount/RBAC)
kubectl delete -f deploy/examples/csi/nvmeof/provisioner.yaml

# Delete the NVMe-oF gateway
kubectl delete -n rook-ceph cephnvmeofgateway.ceph.rook.io my-nvmeof

# Delete the block pool (optional)
kubectl delete -n rook-ceph cephblockpool.ceph.rook.io nvmeof
```

## References

- [Ceph NVMe-oF Documentation](https://docs.ceph.com/en/latest/rbd/nvmeof-overview/)
- [Ceph NVMe-oF Initiator Guide](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux/)
- [Ceph CSI NVMe-oF Support](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/nvme-of.md)
