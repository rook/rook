---
title: Two-Node Cluster
---

# Two-Node Cluster (experimental)

This guide provides installation instructions for setting up a two-node Kubernetes cluster with Rook using DRBD (Distributed Replicated Block Device) for storage replication for one of the Ceph mons.

For architecture details and design rationale, see the [Two-Node Fencing Design Document](https://github.com/rook/rook/blob/master/design/two-node-fencing.md).

## Prerequisites

Before starting the installation:

- **2-node Kubernetes cluster** with fencing configured (e.g., OpenShift with DualReplica topology)
- **Raw block devices** on each node (SSD-class, same size, no filesystem)

## Installation Steps

### Step 1: Install DRBD Kernel Modules

DRBD kernel modules must be installed and loaded on both nodes before running the setup script.

Follow the DRBD installation documentation for your distribution and install `drbd & drbd_transport_tcp`:

- **[DRBD 9.0 Installation Guide](https://linbit.com/drbd-user-guide/drbd-guide-9_0-en/#ch-install-packages)** - Official installation guide
- **[DRBD User's Guide](https://linbit.com/drbd-user-guide/)** - Complete documentation
- **[DRBD GitHub Repository](https://github.com/LINBIT/drbd)** - Source code and build instructions

#### Verify Installation

On both nodes, verify DRBD modules are loaded:

```bash
# Check loaded modules
lsmod | grep drbd

# Expected output:
# drbd_transport_tcp     16384  0
# drbd                  614400  1 drbd_transport_tcp

# Check DRBD version
drbdadm --version
```

### Step 2: Identify Block Devices

Use the setup script to list available block devices on both nodes:

```bash
./deploy/examples/drbd-setup-experimental.sh -l
```

Example output:

```text
=== Block devices (node0=node-0, node1=node-1) ===
Use the PATH column (e.g. -d /dev/sdb or -d0 / -d1 per-node paths).

--- node-0 ---
NAME   PATH      SIZE    ROTA TYPE FSTYPE
sdb    /dev/sdb  10G    0    disk        <-- Use this

--- node-1 ---
NAME   PATH      SIZE    ROTA TYPE FSTYPE
sdb    /dev/sdb  10G    0    disk        <-- Use this

Same path on both nodes: -d <path>
Different paths (same size): -d0 <path0> -d1 <path1>
```

**Requirements for backing devices**:

- Must be SSD/NVMe (ROTA=0, non-rotational)
- Can be a partition of the disk
- Same size on both nodes
- No existing filesystem (FSTYPE empty)
- Writable (not read-only)
- Volume or partition is not larger than 10Gi

### Step 3: Run DRBD Setup Script

The setup script configures DRBD, performs initial sync, and creates the auto-start DaemonSet.

#### Same Device Path on Both Nodes

```bash
./deploy/examples/drbd-setup-experimental.sh -d /dev/sdb
```

#### Different Device Paths Per Node

```bash
./deploy/examples/drbd-setup-experimental.sh -d0 /dev/sdb -d1 /dev/sdc
```

**Available options**:

`./deploy/examples/drbd-setup-experimental.sh -h`

!!! note
    Network connectivity on DRBD replication port (default: 7794) between nodes, we can customise this in the script.

### Step 4: Verify DRBD Setup

Check auto-start DaemonSet:

```bash
kubectl get daemonset drbd-autostart -n rook-ceph
kubectl get pods -n rook-ceph -l app=drbd-autostart
```

Check for the drbd-configure configmap

```bash
kubectl get cm -n rook-ceph drbd-configure -o yaml
```

### Step 5: Deploy Rook CephCluster

Create the CephCluster CR with floating monitor configuration:

Apply the cluster:

```bash
kubectl apply -f deploy/examples/cluster-tnf.yaml
```

### Step 6: Verify Ceph Cluster

Check monitor pods:

```bash
kubectl get pods -n rook-ceph -l app=rook-ceph-mon
```

Expected: 3 monitor pods (mon-a, mon-b, mon-c)

Check Ceph status:

```bash
kubectl exec -n rook-ceph deploy/rook-ceph-tools -- ceph status
```

Expected output:

```text
    cluster:
        id:     <cluster-id>
        health: HEALTH_OK

    services:
        mon: 3 daemons, quorum a,b,c (age ...)
        ...
```

## Configuration Summary

The setup creates:

- **DRBD resource** (`r0`) with Protocol C synchronous replication
- **DRBD device** (`/dev/drbd0`) with XFS filesystem
- **Auto-start DaemonSet** (`drbd-autostart`) in `rook-ceph` namespace
- **Configuration ConfigMap** (`drbd-configure`) in `rook-ceph` namespace
- **Two pinned monitors** (one per node) with local storage
- **One floating monitor** (`mon-c`) backed by DRBD

## Two-Node Cluster Recommendations

Configure pools with **replica-2**, [pool-tnf.yaml](https://github.com/rook/rook/blob/master/deploy/examples/pool-tnf.yaml)

## Troubleshooting

### DRBD Modules Not Loaded

**Check**:

```bash
lsmod | grep drbd
```

**Fix**: Install DRBD kernel modules (see Step 1)

### DRBD Sync Not Progressing

**Check network connectivity**:

```bash
nc -zv <peer-node-ip> 7794
```

**Check DRBD status**:

```bash
drbdadm status r0
```

### Floating Monitor Not Starting

**Check DRBD is Secondary on both nodes**:

```bash
drbdadm role r0
# Expected: Secondary/Secondary
```

**Check ConfigMap**:

```bash
kubectl get configmap drbd-configure -n rook-ceph -o yaml
```

### Re-run Setup Script

The script is idempotent and safe to re-run:

```bash
./deploy/examples/drbd-setup-experimental.sh -d /dev/sdb
```
