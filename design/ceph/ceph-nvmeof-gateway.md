# Ceph NVMe-oF Gateway Support

## Overview

This design document proposes adding NVMe over Fabrics (NVMe-oF) gateway support to Rook Ceph, enabling RBD volumes to be exposed and accessed from outside the Kubernetes cluster via the NVMe/TCP protocol. 

Currently, Rook Ceph provides excellent support for RBD disks through CSI drivers, but lacks a mechanism to expose RBD volumes to clients outside the Kubernetes cluster. With Ceph's deprecation of iSCSI gateway support and the introduction of NVMe-oF gateway functionality, there is an opportunity to provide external block storage access through the use of NVMeOF protocol.

### Goals

- Enable RBD volumes to be accessible from outside the Kubernetes cluster via NVMe/TCP protocol using rook ceph operator

## Proposal:

Rook will have CephNVMeOFGateway CRD for handling the communication b/w Block PVC in the k8s cluster and the client. 

## Prerequisite

- **Ceph Version**: Tentacle (v20) or later with NVMe-oF gateway support
- **RBD Pools and PVCs configured**: Pre-configured RBD pools with appropriate replication/erasure coding and Existing RBD storage classes for volume provisioning.

## CephNVMeOFGateway CRD

### Gateway Configuration

- **Group**: `gateway-group-1` - Logical grouping of gateways
- **Replicas**: `2` - Two gateway instances for HA
- **Port**: `5500` - NVMe-oF service port

### Placement Strategy

```yaml

yaml
nodeAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    nodeSelectorTerms:
    - matchExpressions:
      - key: role
        operator: In
        values: ["osd-node"]

```

- Ensures gateways run only on nodes labeled `role=osd-node`
- Co-locates gateways with OSD storage for optimal performance

### NVMe Subsystem Configuration

- **NQN**: `nqn.2016-06.io.spdk:cnode1` - Unique subsystem identifier
- **Host Access**: Restricted to specific client NQN
- **Security**: `allowAnyHost: false` - Only authorized hosts can connect

### Storage Mapping

- **Backend**: RBD image `external-volume-1` from pool `replicapool`
- **Protocol**: NVMe namespace exposed over TCP/IP fabric with port 5500

### Example CephNVMeOFGateway Resource

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rbd-pool
  namespace: rook-ceph # namespace:cluster
spec:
  name: replicapool
  replicated:
    size: 3
    requireSafeReplicaSize: false
---
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: nvmeof-gateway
  namespace: rook-ceph
spec:
  gatewayGroup: "gateway-group-1"
  replicas: 2
  port: 5500
  placement:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: role
            operator: In
            values: ["osd-node"]
  resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
  subsystems:
  - nqn: "nqn.2016-06.io.spdk:cnode1"
    allowAnyHost: false
    hosts:
    - "nqn.2014-08.org.nvmexpress:uuid:2b1b2e3a-41e2-4d53-8d70-7b7c2f9d8c2b"
    namespaces:
    - rbd:
        pool: "replicapool"
        image: "external-volume-1"
```

### Controller Architecture

The NVMe-oF gateway controller will follow Rook's established patterns:

```
pkg/operator/ceph/nvmeof/
├── controller.go          # Main controller logic
├── gateway.go            # Gateway pod management
├── config.go             # Configuration management
├── service.go            # Service creation
├── monitor.go            # Health monitoring
└── spec.go              # Spec validation
```

The target aka the client can use the steps present in Ceph documentation to connect with the nvme-of target created from rook-ceph cluster.
