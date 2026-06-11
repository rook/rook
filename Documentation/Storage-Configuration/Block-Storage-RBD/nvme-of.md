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

## Step 1: Create the NVMe-oF Gateway

The `CephNVMeOFGateway` CRD manages the NVMe-oF gateway infrastructure. The operator will automatically create the following resources:

- **Service**: One per gateway instance for service discovery
- **Deployment**: One per gateway instance running the NVMe-oF gateway daemon

NVMe-oF uses two separate pools:

- **Metadata pool (`.nvmeof`)**: An internal pool that stores gateway state (subsystem and namespace definitions). Created with `spec.name: .nvmeof`.
- **Data pool**: A standard CephBlockPool that holds the actual RBD image data. See [Step 3](#step-3-create-the-data-pool).

Create the gateway and the `.nvmeof` metadata pool:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: builtin-nvmeof
  namespace: rook-ceph
spec:
  name: .nvmeof
  failureDomain: host
  replicated:
    size: 3
---
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: nvmeof
  namespace: rook-ceph
spec:
  # ANA (Asymmetric Namespace Access) group name
  group: group-a
  # Number of gateway instances to run (2 recommended for HA)
  instances: 2
  hostNetwork: false
```

Apply the gateway configuration:

```console
kubectl create -f deploy/examples/nvmeof.yaml
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

## Step 2: Deploy the NVMe-oF CSI Driver via CSI Operator

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

## Step 3: Create the Data Pool

Create a CephBlockPool that will be used by the StorageClass to store data:

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

```console
kubectl create -f deploy/examples/csi/nvmeof/nvmeof-pool.yaml
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
      },
      {
        "hostname": "rook-ceph-nvmeof-nvmeof-b"
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

## Step 5: Create a VolumeAttributesClass (Optional)

A `VolumeAttributesClass` defines mutable volume parameters such as host access control.
This is required when external clients need to connect to NVMe-oF volumes, as it specifies
which host NQNs are allowed to access the volume.

Create the `VolumeAttributesClass` with the allowed host NQNs.
See the [example VolumeAttributesClass](https://github.com/rook/rook/blob/master/deploy/examples/csi/nvmeof/volume-attributes-class.yaml) for reference.

```console
kubectl create -f deploy/examples/csi/nvmeof/volume-attributes-class.yaml
```

## Step 6: Create a PersistentVolumeClaim

Create a PVC using the NVMe-oF storage class:

```console
kubectl create -f deploy/examples/csi/nvmeof/pvc.yaml
```

Verify the PVC is bound:

```console
kubectl get pvc nvmeof-volume
```

**Example Output**

```console
NAME             STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
nvmeof-volume    Bound    pvc-b4108580-5cfa-46d3-beff-320088a5bf3c   1Gi        RWO            ceph-nvmeof    20m
```

## Step 7: Create a Pod

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

## Step 8: Accessing Volumes from External Clients

External clients outside the Kubernetes cluster connect to NVMe-oF
volumes using standard NVMe-oF initiators. This requires a dedicated
StorageClass, a PVC with a VolumeAttributesClass for host access
control, and LoadBalancer services to expose the gateways.

### Create External StorageClass and PVC

The external StorageClass does not include `subsystemNQN`. Each PVC is
automatically placed in its own subsystem, and external clients discover
the subsystem NQN dynamically via `nvme discover`.

Create the external StorageClass and PVC.
See the [example external StorageClass](https://github.com/rook/rook/blob/master/deploy/examples/csi/nvmeof/storageclass-external.yaml)
and [example external PVC](https://github.com/rook/rook/blob/master/deploy/examples/csi/nvmeof/pvc-external.yaml) for reference.

```console
kubectl create -f deploy/examples/csi/nvmeof/storageclass-external.yaml
kubectl create -f deploy/examples/csi/nvmeof/pvc-external.yaml
```

Verify the PVC is bound:

```console
kubectl get pvc nvmeof-external-volume
```

**Example Output**

```console
NAME                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS           VOLUMEATTRIBUTESCLASS   AGE
nvmeof-external-volume   Bound    pvc-bbad520d-fa7d-476c-904e-a8da2b6476ab   1Gi        RWO            ceph-nvmeof-external   nvmeof-external-hosts   19s
```

### Prerequisites for External Clients

- **nvme-cli**: The `nvme` command-line tool must be installed
- **Network Access**: The client must be able to reach the gateway LoadBalancer IP
- **Host NQN**: The client's host NQN must be listed in the `VolumeAttributesClass` (see Step 5)

Install `nvme-cli` and load the required kernel module:

```bash
# Fedora / RHEL
sudo dnf install nvme-cli

# Load NVMe-oF/TCP transport
sudo modprobe nvme-tcp
```

### Create LoadBalancer Services

External clients need LoadBalancer services to reach the NVMe-oF gateways from outside the cluster.
Create one LoadBalancer per gateway instance so each gateway has a dedicated external endpoint.
See the [example LoadBalancer services](https://github.com/rook/rook/blob/master/deploy/examples/csi/nvmeof/nvmeof-lb.yaml) for reference.

Create the services:

```console
kubectl create -f deploy/examples/csi/nvmeof/nvmeof-lb.yaml
```

Get the external IPs:

```console
kubectl get service -n rook-ceph nvmeof-lb-a nvmeof-lb-b
```

**Example Output**

```console
NAME           TYPE           CLUSTER-IP      EXTERNAL-IP       PORT(S)                         AGE
nvmeof-lb-a    LoadBalancer   10.96.100.50    192.168.1.100     4420:31334/TCP,8009:31235/TCP    60s
nvmeof-lb-b    LoadBalancer   10.96.100.51    192.168.1.101     4420:31336/TCP,8009:31237/TCP    60s
```

### Connect from the External Client

The connection process has two stages: first **discover** the available subsystems
via the discovery port (8009), then **connect** to the discovered subsystem
via the I/O port (4420).

#### Step 1: Discover subsystems

Run a discovery against either gateway LoadBalancer address using the discovery port (8009):

```bash
sudo nvme discover -t tcp -a <GW-A-EXTERNAL-IP> -s 8009
```

Replace `<GW-A-EXTERNAL-IP>` with one of the LoadBalancer external IPs from the service output above.

The discovery log returns the target subsystem NQN and available transport addresses:

```console
Discovery Log Number of Records 2, Generation counter 7
=====Discovery Log Entry 0======
trtype:  tcp
subtype: nvme subsystem
trsvcid: 4420
subnqn:  nqn.2016-06.io.ceph:subsystem.0001-0009-rook-ceph-0000000000000003-...
traddr:  0.0.0.0
```

#### Step 2: Connect to the gateway

Use the subsystem NQN from the discovery output to connect through each gateway:

```bash
sudo nvme connect -t tcp \
  -n <SUBSYSTEM-NQN> \
  -a <GW-A-EXTERNAL-IP> -s 4420

sudo nvme connect -t tcp \
  -n <SUBSYSTEM-NQN> \
  -a <GW-B-EXTERNAL-IP> -s 4420
```

Verify the connection:

```bash
sudo nvme list-subsys
```

### Format and Mount

```bash
sudo mkfs.ext4 /dev/nvme1n1
sudo mkdir -p /mnt/nvmeof
sudo mount /dev/nvme1n1 /mnt/nvmeof
sudo chown $(whoami):$(whoami) /mnt/nvmeof
```

### Disconnect

To disconnect the NVMe-oF volume from the external client:

```bash
sudo umount /mnt/nvmeof
sudo nvme disconnect-all
```

## High Availability

The example (`nvmeof.yaml`) configures `instances: 2` for high availability.
When running multiple gateway instances, update the StorageClass `listeners` to include all gateway
deployment hostnames for multipath/HA:

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

## Teardown

!!! warning
    Deleting the PVC will also delete the underlying RBD image and NVMe namespace. Ensure you have backups if needed.

To clean up all the artifacts created:

```console
# Delete the test pod
kubectl delete -f deploy/examples/csi/nvmeof/pod.yaml

# Delete the in-cluster PVC
kubectl delete pvc nvmeof-volume

# Delete external resources (if created)
kubectl delete pvc nvmeof-external-volume
kubectl delete volumeattributesclass nvmeof-external-hosts
kubectl delete service -n rook-ceph nvmeof-lb-a nvmeof-lb-b
kubectl delete storageclass ceph-nvmeof-external

# Delete the in-cluster StorageClass
kubectl delete storageclass ceph-nvmeof

# Delete the NVMe-oF CSI driver
kubectl delete -f deploy/examples/csi/nvmeof/driver.yaml

# Delete the NVMe-oF gateway and its metadata pool
kubectl delete -f deploy/examples/nvmeof.yaml

# Delete the data pool
kubectl delete -f deploy/examples/csi/nvmeof/nvmeof-pool.yaml
```

## References

- [Ceph NVMe-oF Documentation](https://docs.ceph.com/en/latest/rbd/nvmeof-overview/)
- [Ceph NVMe-oF Initiator Guide](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux/)
- [Ceph CSI NVMe-oF Support](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/nvme-of.md)
