---
title: NVMe-oF Block Storage
---
# NVMe-oF Block Storage

**This feature is experimental**

NVMe over Fabrics (NVMe-oF) allows RBD volumes to be exposed and accessed via the NVMe/TCP protocol. This enables both Kubernetes pods within the cluster and external clients outside the cluster to connect to Ceph block storage using standard NVMe-oF initiators, providing high-performance block storage access over the network.

## Goals

The NVMe-oF integration in Rook serves two primary purposes:

1. **In-Cluster Consumption**: Pods inside the Kubernetes cluster can consume storage via the NVMe-oF protocol, providing an alternative to traditional RBD mounts with potential performance benefits for certain workloads.

2. **External Client Access**: Rook serves as a backend for external clients outside the cluster, enabling non-Kubernetes workloads to access Ceph block storage through standard NVMe-oF initiators. This allows organizations to leverage their Ceph storage infrastructure for both containerized and traditional workloads.

Both use cases share a common foundation (gateway, CSI driver, and data pool) and then diverge.
Follow the [Common Setup](#common-setup) first, then continue with either
[In-Cluster Deployment](#in-cluster-deployment-for-pods) or
[External Client Deployment](#external-client-deployment-via-loadbalancer) (or both).

For more background and design details, see the [NVMe-oF gateway design doc](https://github.com/rook/rook/blob/master/design/ceph/ceph-nvmeof-gateway.md).
For the Ceph-CSI NVMe-oF design proposal, see the [ceph-csi NVMe-oF proposal](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/nvme-of.md).

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart Guide](../../Getting-Started/quickstart.md).

### Requirements

- **Ceph Version**: Ceph v20 (Tentacle) or later

---

## Common Setup

These steps are required for **both** in-cluster and external deployments.

### Step 1: Create the NVMe-oF Gateway

The `CephNVMeOFGateway` CRD manages the NVMe-oF gateway infrastructure. The operator will automatically create the following resources:

- **Service**: One per gateway instance for service discovery
- **Deployment**: One per gateway instance running the NVMe-oF gateway daemon
- **NetworkPolicy**: One per gateway, restricting access to the gateway's gRPC API (see [Gateway Network Access](#gateway-network-access))

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

### Step 2: Deploy the NVMe-oF CSI Driver via CSI Operator

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

### Step 3: Create the Data Pool

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

---

## In-Cluster Deployment (for Pods)

This section covers consuming NVMe-oF volumes from pods inside the Kubernetes cluster
via the NVMe-oF CSI driver.

### Step 4: Create the StorageClass

Create a StorageClass that uses the NVMe-oF CSI driver.
The `subsystemNQN` parameter places all in-cluster PVCs into a shared subsystem,
and the `listeners` list tells the CSI node plugin which gateways to connect to for I/O.

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

### Step 5: Create a PersistentVolumeClaim

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

### Step 6: Create a Pod

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

---

## External Client Deployment (via LoadBalancer)

This section covers exposing NVMe-oF volumes to clients **outside** the Kubernetes
cluster. External clients connect using standard NVMe-oF initiators (`nvme-cli`)
through LoadBalancer services.

### Prerequisites for External Clients

- **nvme-cli**: The `nvme` command-line tool must be installed on the client
- **Network Access**: The client must be able to reach the gateway LoadBalancer IP
- **Host NQN**: The client's host NQN must be listed in the `VolumeAttributesClass`

Install `nvme-cli` and load the required kernel module on the external client:

```bash
# Fedora / RHEL
sudo dnf install nvme-cli

# Load NVMe-oF/TCP transport
sudo modprobe nvme-tcp
```

### Step 4: Create a VolumeAttributesClass

A `VolumeAttributesClass` defines mutable volume parameters such as host access control.
It specifies which host NQNs are allowed to access the volume.

Create the `VolumeAttributesClass` with the allowed host NQNs:

```console
kubectl create -f deploy/examples/csi/nvmeof/volume-attributes-class.yaml
```

### Step 5: Create the External StorageClass and PVC

The external StorageClass does **not** include `subsystemNQN`. Each PVC is
automatically placed in its own subsystem, and external clients discover
the subsystem NQN dynamically via `nvme discover`.

Create the external StorageClass and PVC:

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

### Step 6: Create LoadBalancer Services

External clients need LoadBalancer services to reach the NVMe-oF gateways from outside the cluster.
Create one LoadBalancer per gateway instance so each gateway has a dedicated external endpoint.

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

### Step 7: Connect from the External Client

The connection process has two stages: first **discover** the available subsystems
via the discovery port (8009), then **connect** to the discovered subsystem
via the I/O port (4420).

#### Discover subsystems

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

#### Connect to the LoadBalancer

Use the subsystem NQN from the discovery output to connect through **all** gateway
LoadBalancers in the gateway group. The host must connect to every gateway to see all namespaces:

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

---

## Gateway Network Access

The gateway's gRPC API manages subsystems, hosts, listeners and namespaces, and it does not
authenticate its callers. The operator therefore creates a NetworkPolicy named
`rook-ceph-nvmeof-<gateway-name>` in the gateway's namespace. It selects the gateway's pods and
permits:

- the gRPC API port (5500, or `[gateway] port` from `nvmeofConfig`) only from the NVMe-oF CSI controller plugin pods in the operator namespace and from the gateway's own pods;
- the monitor port, which the gateway always binds one below the gRPC API port, only from the gateway's own pods;
- the I/O port (`spec.ports.ioPort`, default 4420), the discovery port (8009, or `[discovery] port` from `nvmeofConfig`) and the Prometheus exporter port (10008, or `[gateway] prometheus_port` from `nvmeofConfig`) from any source, inside or outside the cluster.

The `spec.ports` values are covered as well, because the gateway's Services publish them. Every
other port of the gateway pods is blocked, including NVMe/TCP listeners created on a port other
than the I/O port. When the gateway uses a configuration from `configMapRef`, ports that are set
only in that configuration are not covered and need an additional NetworkPolicy.

!!! note
    A NetworkPolicy only takes effect if the cluster's network plugin (CNI) enforces NetworkPolicy.

To find the CSI controller plugin, the operator looks in its own namespace for `csi.ceph.io`
`Driver` CRs named `nvmeof.csi.ceph.com` or `<prefix>.nvmeof.csi.ceph.com`, such as the one in
`deploy/examples/csi/nvmeof/driver.yaml`. For each one it admits the pods of the controller plugin
Deployment `<driver-name>-ctrlplugin`, selected by their `app` label. If no such `Driver` exists,
the policy admits only the gateway's own pods to the gRPC API, the operator logs a warning, and
NVMe-oF volumes cannot be provisioned until the `Driver` is created.

Where the cluster source-NATs pod-to-Service traffic to the node address, for example with
kube-proxy `--masquerade-all`, the CSI controller plugin's connections may not match the policy,
and an additional NetworkPolicy that admits the node CIDRs on the gRPC API port is needed.

The operator does not create the policy, and removes one it created earlier, when:

- the gateway uses host networking (`hostNetwork: true` on the `CephNVMeOFGateway`, or host networking inherited from the `CephCluster`), because a NetworkPolicy does not apply to host network pods;
- a CSI controller plugin Deployment runs with host networking (for example with `controllerPlugin.hostNetwork: true` in the `Driver` CR). Its connections then come from node addresses, which a pod selector cannot match. The operator logs a warning in this case.

### Allowing Other Clients

NetworkPolicies are additive: a connection is allowed when any policy that selects the gateway pod
allows it. To let another client reach the gateway, such as `nvmeof-cli` running in another pod,
create an additional NetworkPolicy in the gateway's namespace rather than editing the operator's
policy, which the operator overwrites on its next reconcile. For example, to let pods labeled
`app: nvmeof-admin` in the `nvmeof-admin` namespace use the gRPC API of the gateway `nvmeof`:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nvmeof-admin-grpc
  namespace: rook-ceph
spec:
  podSelector:
    matchLabels:
      app: rook-ceph-nvmeof
      app.kubernetes.io/part-of: nvmeof
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: nvmeof-admin
          podSelector:
            matchLabels:
              app: nvmeof-admin
      ports:
        - protocol: TCP
          port: 5500
```

---

## Teardown

!!! warning
    Deleting a PVC will also delete the underlying RBD image and NVMe namespace. Ensure you have backups if needed.

### In-Cluster Teardown

```console
# Delete the test pod
kubectl delete -f deploy/examples/csi/nvmeof/pod.yaml

# Delete the PVC
kubectl delete pvc nvmeof-volume

# Delete the StorageClass
kubectl delete storageclass ceph-nvmeof
```

### External Teardown

Disconnect the NVMe-oF volume from the external client:

```bash
sudo umount /mnt/nvmeof
sudo nvme disconnect-all
```

Then delete the Kubernetes resources:

```console
# Delete the external PVC
kubectl delete pvc nvmeof-external-volume

# Delete the VolumeAttributesClass
kubectl delete volumeattributesclass nvmeof-external-hosts

# Delete the LoadBalancer services
kubectl delete service -n rook-ceph nvmeof-lb-a nvmeof-lb-b

# Delete the external StorageClass
kubectl delete storageclass ceph-nvmeof-external
```

### Common Teardown

After removing the mode-specific resources above, clean up the shared infrastructure:

```console
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
