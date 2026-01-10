---
title: NVMe-oF Block Storage
---

NVMe over Fabrics (NVMe-oF) allows RBD volumes to be exposed and accessed via the NVMe/TCP protocol. This enables both Kubernetes pods within the cluster and external clients outside the cluster to connect to Ceph block storage using standard NVMe-oF initiators, providing high-performance block storage access over the network.

## Goals

The NVMe-oF integration in Rook serves two primary purposes:

1. **In-Cluster Consumption**: Pods inside the Kubernetes cluster can consume storage via NVMe-oF protocol, providing an alternative to traditional RBD mounts with potential performance benefits for certain workloads.

2. **External Client Access**: Rook serves as a backend for external clients outside the cluster, enabling non-Kubernetes workloads to access Ceph block storage through standard NVMe-oF initiators. This allows organizations to leverage their Ceph storage infrastructure for both containerized and traditional workloads.

Both use cases are supported, allowing you to choose the appropriate access method based on your specific requirements and deployment scenarios.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart Guide](../../Getting-Started/quickstart.md).

### Requirements

- **Ceph Version**: Ceph v20 (Tentacle) or later with NVMe-oF gateway support
- For more background and design details, see the [NVMe-oF gateway design doc](../../../design/ceph/ceph-nvmeof-gateway.md).

## Step 1: Create a Ceph Block Pool

Before creating the NVMe-oF gateway, you need to create a CephBlockPool that will be used by the gateway:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: nvmeofpool
  namespace: rook-ceph
spec:
  failureDomain: osd
  replicated:
    size: 1
    # Disallow setting pool with replica 1, this could lead to data loss without recovery.
    # Make sure you're *ABSOLUTELY CERTAIN* that is what you want
    # requireSafeReplicaSize: false
```

!!! warning
    The example above uses `size: 1` for simplicity. In production environments, use at least `size: 3` with `failureDomain: host` for data durability and high availability.

Create the pool:

```console
kubectl create -f nvmeof-pool.yaml
```

## Step 2: Create the NVMe-oF Gateway

The `CephNVMeOFGateway` CRD manages the NVMe-oF gateway infrastructure. The operator will automatically create the following resources:

- **ConfigMap**: Contains the gateway configuration (if `configMapRef` is not specified)
- **Service**: One per gateway instance for service discovery
- **Deployment**: One per gateway instance running the NVMe-oF gateway daemon

Create the gateway:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: my-nvmeof
  namespace: rook-ceph
spec:
  # Pool name that will be used by the NVMe-oF gateway
  pool: nvmeofpool
  # ANA (Asymmetric Namespace Access) group name
  group: mygroup
  # Number of gateway instances to run
  instances: 1
  # Optional: Custom ConfigMap reference (if not specified, operator will create one)
  # configMapRef: ""
  # Optional: Resource requirements for the gateway pods
  # resources:
  #   limits:
  #     cpu: "500m"
  #     memory: "512Mi"
  #   requests:
  #     cpu: "200m"
  #     memory: "256Mi"
  # Optional: Priority class name
  # priorityClassName: ""
  # Optional: Liveness probe configuration
  # livenessProbe:
  #   disabled: false
  # Optional: Annotations to apply to the deployment
  # annotations: {}
  # Optional: Labels to apply to the deployment
  # labels: {}
  # Optional: Placement configuration
  # placement: {}
  # Optional: Host networking configuration (default: false, uses pod network)
  # Gateways work better when on pod network and exposed via ingress/service
  hostNetwork: false
  # Optional: Port configuration (defaults shown below)
  # ports:
  #   ioPort: 4420
  #   gatewayPort: 5500
  #   monitorPort: 5499
  #   discoveryPort: 8009
```

Apply the gateway configuration:

```console
kubectl create -f nvmeof-gateway.yaml
```

Verify the gateway is running:

```console
kubectl get deployments -n rook-ceph | grep nvmeof
```

!!! example "Example Output"
    ```console
    NAME                                    READY   UP-TO-DATE   AVAILABLE   AGE
    rook-ceph-nvmeof-my-nvmeof-0           1/1     1            1           2m
    ```

## Step 3: Verify Gateway Resources

Check that the operator has created the necessary resources:

### Check the Service

```console
kubectl get service -n rook-ceph rook-ceph-nvmeof-my-nvmeof-0
```

!!! example "Example Output"
    ```console
    NAME                           TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)                               AGE
    rook-ceph-nvmeof-my-nvmeof-0   ClusterIP   10.99.212.218   <none>        4420/TCP,5500/TCP,5499/TCP,8009/TCP   5m
    ```

Note the `CLUSTER-IP` address (e.g., `10.99.212.218`) - this will be used as the `nvmeofGatewayAddress` in the StorageClass.

### Check the Gateway Pod

```console
kubectl get pods -n rook-ceph -l app=rook-ceph-nvmeof -o wide
```

!!! example "Example Output"
    ```console
    NAME                                            READY   STATUS    RESTARTS   AGE   IP            NODE
    rook-ceph-nvmeof-my-nvmeof-0-7c785ff5f6-6wxrj   1/1     Running   0          5m    10.244.0.16   minikube
    ```

Note the pod's:
- **IP address** (e.g., `10.244.0.16`) - this will be used as the listener address
- **Hostname** (e.g., `rook-ceph-nvmeof-my-nvmeof-0-7c785ff5f6-6wxrj`) - this will be used as the listener hostname

### Check the ConfigMap

```console
kubectl get configmap -n rook-ceph rook-ceph-nvmeof-my-nvmeof-0-config -o yaml
```

The ConfigMap contains the gateway configuration that will be used by the gateway pods.

## Step 4: Deploy the NVMe-oF CSI Driver

The NVMe-oF CSI driver handles dynamic provisioning of volumes. You need to deploy the CSI provisioner with the NVMe-oF driver.

Deploy the NVMe-oF CSI provisioner from the example manifest:

```console
kubectl apply -f deploy/examples/csi/nvmeof/provisioner.yaml
```

!!! important
    The example manifest assumes your Ceph cluster is in the `rook-ceph` namespace and that the Rook operator has created the `rook-ceph-csi-config` ConfigMap. Update the namespace and pin `quay.io/ceph/cephcsi` to an appropriate tag for your environment.

Verify the provisioner is running:

```console
kubectl get deployment -n rook-ceph csi-nvmeofplugin-provisioner
```

## Step 5: Create the StorageClass

Create a StorageClass that uses the NVMe-oF CSI driver. You'll need to gather the following information from the gateway:

1. **nvmeofGatewayAddress**: The ClusterIP of the gateway service (from Step 3)
2. **nvmeofGatewayPort**: The gateway port (default: 5500)
3. **listeners**: A JSON array containing listener information for each gateway pod

Based on the gateway pod information from Step 3, create the StorageClass:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof
parameters:
  clusterID: rook-ceph
  pool: nvmeofpool
  subsystemNQN: nqn.2016-06.io.spdk:cnode1.rook-ceph
  # Management API - talks to gateway to create subsystems/namespaces
  nvmeofGatewayAddress: "10.99.212.218"  # Replace with your gateway service ClusterIP
  nvmeofGatewayPort: "5500"
  # Data Plane - worker nodes connect here for actual I/O
  # List ALL gateway pods for HA and multipath
  listeners: |
    [
      {
        "address": "10.244.0.16",
        "port": 4420,
        "hostname": "rook-ceph-nvmeof-my-nvmeof-0-7c785ff5f6-6wxrj"
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

!!! note
    Replace the values in the StorageClass with your actual gateway information:
    - `nvmeofGatewayAddress`: Use the ClusterIP from `kubectl get service -n rook-ceph rook-ceph-nvmeof-my-nvmeof-0`
    - `listeners`: Use the pod IP and hostname from `kubectl get pods -n rook-ceph -l app=rook-ceph-nvmeof -o wide`

!!! tip
    For high availability with multiple gateway instances, add multiple entries to the `listeners` array, one for each gateway pod.

Create the StorageClass:

```console
kubectl create -f nvmeof-storageclass.yaml
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
kubectl create -f nvmeof-pvc.yaml
```

Verify the PVC is bound:

```console
kubectl get pvc nvmeof-external-volume
```

!!! example "Example Output"
    ```console
    NAME                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    nvmeof-external-volume   Bound    pvc-b4108580-5cfa-46d3-beff-320088a5bf3c   128Mi      RWO            ceph-nvmeof    20m
    ```

## Step 7: Accessing Volumes via NVMe-oF

Once the PVC is created and bound, the volume is available via NVMe-oF. The volume can be accessed by both Kubernetes pods within the cluster and external clients outside the cluster.

### Access from Kubernetes Pods

Kubernetes pods can consume NVMe-oF volumes by mounting the PVC directly. The CSI driver handles the NVMe-oF connection automatically when the pod mounts the volume.

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
- `<subsystem-nqn>` with the NQN from your StorageClass (e.g., `nqn.2016-06.io.spdk:cnode1.rook-ceph`)
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
2. **Update StorageClass Listeners**: Add all gateway pod IPs and hostnames to the `listeners` array
3. **Load Balancing**: The Kubernetes service automatically load-balances connections across gateway instances

Example with multiple instances:

```yaml
spec:
  instances: 2
  # ... other settings
```

Then update the StorageClass `listeners` to include all gateway pods:

```yaml
listeners: |
  [
    {
      "address": "10.244.0.16",
      "port": 4420,
      "hostname": "rook-ceph-nvmeof-my-nvmeof-0-xxx"
    },
    {
      "address": "10.244.0.17",
      "port": 4420,
      "hostname": "rook-ceph-nvmeof-my-nvmeof-1-yyy"
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

To clean up all the artifacts created:

```console
# Delete the PVC
kubectl delete pvc nvmeof-external-volume

# Delete the StorageClass
kubectl delete storageclass ceph-nvmeof

# Delete the CSI provisioner
kubectl delete deployment -n rook-ceph csi-nvmeofplugin-provisioner

# Delete the RBAC resources
kubectl delete -f nvmeof-csi-rbac.yaml

# Delete the NVMe-oF gateway
kubectl delete -n rook-ceph cephnvmeofgateway.ceph.rook.io my-nvmeof

# Delete the block pool (optional)
kubectl delete -n rook-ceph cephblockpool.ceph.rook.io nvmeofpool
```

!!! warning
    Deleting the PVC will also delete the underlying RBD image and NVMe namespace. Ensure you have backups if needed.

## References

- [Ceph NVMe-oF Documentation](https://docs.ceph.com/en/latest/rbd/nvmeof-overview/)
- [Ceph NVMe-oF Initiator Guide](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux/)
- [Ceph CSI NVMe-oF Support](https://github.com/ceph/ceph-csi)
