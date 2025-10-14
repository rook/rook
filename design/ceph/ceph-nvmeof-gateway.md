# Ceph NVMe-oF Gateway Support

## Overview

This design document proposes adding NVMe over Fabrics (NVMe-oF) gateway support to Rook Ceph, enabling RBD volumes to be exposed and accessed from outside the Kubernetes cluster via the NVMe/TCP protocol.

Currently, Rook Ceph provides excellent support for RBD disks through CSI drivers, but lacks a mechanism to expose RBD volumes to clients outside the Kubernetes cluster. With Ceph's deprecation of iSCSI gateway support and the introduction of NVMe-oF gateway functionality, there is an opportunity to provide external block storage access through the use of NVMeOF protocol.

### Goals

- Enable RBD volumes to be accessible from outside the Kubernetes cluster via NVMe/TCP protocol using rook ceph operator.
- Integrate with Ceph CSI for dynamic provisioning of NVMe namespaces.

## Proposal:

Rook will have CephNVMeOFGateway CRD for handling the communication b/w Block PVC in the k8s cluster and the client.

### Architecture Overview

The NVMe-oF implementation separates concerns between infrastructure management and storage provisioning:

Rook Operator: Manages NVMe-oF gateway pod lifecycle, scaling, and health monitoring
Ceph CSI Driver: Handles dynamic provisioning, subsystem creation, and NVMe namespace management
NVMe-oF Gateways: Serve NVMe-oF protocol and manage RBD backend connections

## Prerequisite

- **Ceph Version**: Tentacle (v20) or later with NVMe-oF gateway support
- **RBD Pools and PVCs configured**: Pre-configured RBD pools with appropriate replication/erasure coding and Existing RBD storage classes for volume provisioning.
- **Ceph CSI**: For dynamic provisioning of NVMe namespace (version yet to be added)
- **Kernel Modules (Initiator) for client nodes**: Client nodes must have the nvme-tcp kernel module loaded and nvme-cli installed for initiator functionality.

## Architecture Diagram


                                         +----------------------+
                                         |  Kubernetes Cluster  |
                                         +----------------------+
                                                    |
                     +------------------------------|------------------------------+
                     |                              |                              |
            +--------v--------+             +-------v-------+             +-------v-------+
            |  Rook Operator   | --------.  |   Ceph Cluster |  .---------| NVMe-oF Gateway |
            +------------------+          \ +----------------+ /          +-----------------+
                                           \        |         /
                                            \       |        /
                                         +-----------v-----------+
                                         |     Storage Layer      |
                                         | +--------------------+ |
                                         | | OSD 1 (Physical)   | |
                                         | +--------------------+ |
                                         +-----------|------------+
                                                     |
                                          +----------v----------+
                                          |   RBD Volume         |
                                          |   (Block Device)      |
                                          +----------|------------+
                                                     |
                                          +----------v----------+
                                          | NVMe-oF Target       |
                                          | (Presents RBD as     |
                                          | NVMe Namespace)      |
                                          +----------|------------+
                                                     |
                                           +---------v---------+
                                           |      Network       |
                                           | TCP/IP or RDMA     |
                                           | (NVMe-oF Protocol) |
                                           +---------|-----------+
                                                     |
                                      +--------------v--------------+
                                      |        Client Node          |
                                      +-----------------------------+
                                      | NVMe-oF Initiator (nvme-cli)|
                                      |                             |
                                      |  Application sees           |
                                      |  /dev/nvmeXnY               |
                                      +-----------------------------+





* Note:  In this document, when we refer to 'NVMe namespace,' we mean the NVMe protocol's concept of a logical block device, not Kubernetes resource namespaces.

### Components Responsibilities

* **Volume Provisioning**: CSI driver creates RBD image and configures it in NVMe-oF gateway.
* **Gateway Management**: Rook operator manages gateway lifecycle, scaling, and health.
* **Load Balancing/k8s Service**: Service provides single endpoint for multiple gateway instances, NVMe namespace exposed over TCP/IP fabric with port 5500.
* **Client Access**: External clients connect via NVMe-oF initiator to access volumes.

## CephNVMeOFGateway CRD

The CephNVMeOFGateway CRD focuses solely on gateway infrastructure deployment, leaving storage provisioning to the CSI driver.

### Gateway Configuration

- **Group**: `gateway-group-1` - Logical grouping of gateways
- **Replicas**: `2` - Two gateway instances for HA



### Example CephNVMeOFGateway Resource

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: nvmeof-gateway
  namespace: rook-ceph
spec:
  gatewayGroup: "gateway-group-1"
  replicas: 2
  placement:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node-role.kubernetes.io/storage
            operator: In
            values: ["true"]
  resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
```
## CSI Integration for Dynamic Provisioning

StorageClass Configuration
The NVMe-oF StorageClass integrates with the CSI driver for dynamic provisioning :

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof
parameters:
  clusterID: rook-ceph
  pool: replicapool
  subsystemNQN: nqn.2016-06.io.ceph:rook-ceph
  nvmeofGatewayAddress: ceph-nvmeof-gateway.rook-ceph.svc.cluster.local
  nvmeofGatewayPort: "5500"
  listenerPort: "4420"
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
``
---yaml
# Example PVC using NVMe-oF
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
      storage: 10Gi
```
Note: This PVC is created solely for CSI driver provisioning. No Kubernetes pod will mount it as the volume is accessed by external NVMe-oF clients.

The target aka the client can than be configured from the procedure provided [Ceph documentation](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux/) for connecting to the NVMe backed Block device.

## High Availability and Load Balancing
### Gateway Groups
Multiple gateways within the same group share configuration and provide high availability :​
- All gateways in a group present the same NVMe subsystems and namespaces
- Minimum of 2 gateways required for HA
- Gateways are load-balanced through Kubernetes service endpoints

### Service Configuration
Rook automatically creates a Kubernetes service for gateway discovery:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ceph-nvmeof-gateway-group-1
  namespace: rook-ceph
  labels:
    gateway-group: gateway-group-1
spec:
  selector:
    app: rook-ceph-nvmeof-gateway
    gateway-group: gateway-group-1
  ports:
  - name: nvmeof-data
    port: 4420
    targetPort: 4420
    protocol: TCP
  - name: management
    port: 5500
    targetPort: 5500
    protocol: TCP
  type: ClusterIP
```

### Client Connection
External clients connect using standard NVMe-oF procedures.
Detailed client setup instructions are available in the Ceph documentation.​
Basic Client Connection Steps:

- Discover available subsystems:

```bash
nvme discover -t tcp -a <gateway-service-ip> -s 5500
```

- Connect to discovered subsystem:
```bash
nvme connect -t tcp -n <nqn> -a <gateway-ip> -s 5500
```

- Access the NVMe namespace as a block device (/dev/nvmeXnY)

### Terminology

**NVMe Namespace:** logical block device (NVMe namespace) that is presented by the NVMe-oF gateway.

**subsystems:** A list of NVMe-oF subsystems to be configured on this gateway group. Each subsystem acts as a container for NVMe namespaces and defines access control.

**nqn:** The NVMe Qualified Name (NQN) for the subsystem (e.g., nqn.2016-06.io.spdk:production). This NQN will be advertised to initiators.

**hosts:** A list of initiator NQNs allowed to connect to this specific subsystem. If empty, any initiator can connect (less secure). This can be dynamically updated by the CSI driver or manually.

### References
https://docs.ceph.com/en/latest/rbd/nvmeof-overview/
https://github.com/ceph/ceph-nvmeof
https://github.com/ceph/ceph-csi/pull/5397/
