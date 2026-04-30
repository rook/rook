# Accessing Ceph NVMe-oF Volumes from External Clients

This guide walks through connecting to a Ceph NVMe-oF volume from an external client
(a machine outside the Kubernetes cluster). The volume is backed by a Ceph RBD
image and served through the NVMe-oF gateway running inside the cluster.

> **Tested on:** OpenShift (OCP) 4.21 on IBM Cloud VPC platform. The steps apply to
> any Kubernetes cluster.

## Architecture Overview

```
┌─────────────────────┐
│   External Client   │
│     (My Linux PC)   │
│                     │
│  /mnt/nvmeof/       │
│    ├── new.txt      │
│    ├── test.txt     │
│    └── ...          │
│                     │
│  nvme-cli           │
│  nvme-tcp module    │
└────────┬────────────┘
         │
         │  NVMe-oF / TCP (port 4420)
         │
         ▼
┌────────────────────────────────────────────────────────────────────┐
│                        IBM Cloud / Internet                        │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │              LoadBalancer Service (Public IPs)               │  │
│  │         169.62.19.103  /  150.240.2.147  :4420               │  │
│  └────────────────────── ───┬───────────────────────────────────┘  │
│                             │                                      │
│  ┌──────────────────────────┴───────────────────────────────────┐  │
│  │                   Kubernetes Cluster                         │  │
│  │                                                              │  │
│  │   ┌─────────────────────────────────────────────────────┐    │  │
│  │   │          NVMe-oF Gateway Pod (SPDK-based)           │    │  │
│  │   │       rook-ceph-nvmeof-nvmeof-a  :4420/:5500        │    │  │
│  │   └──────────────────────┬──────────────────────────────┘    │  │
│  │                          │                                   │  │
│  │   ┌──────────────────────┴───────────────────────────── ─┐   │  │
│  │   │              Ceph Cluster (3 OSDs)                   │   │  │
│  │   │                                                      │   │  │
│  │   │   ┌──────────────────────────────────────────────┐   │   │  │
│  │   │   │    RBD Image (pool: nvmeof, replicated x3)   │   │   │  │
│  │   │   │    csi-vol-d47a1278-...  (128 MB)            │   │   │  │
│  │   │   └──────────────────────────────────────────────┘   │   │  │
│  │   └──────────────────────────────────────────────────────┘   │  │
│  │                                                              │  │
│  │   ┌──────────────────────────────────────────────────────┐   │  │
│  │   │   Debug Pod (nvme-client-test) - verifies data       │   │  │
│  │   │   mount -o ro /dev/nvme0n1 /mnt/nvmeof               │   │  │
│  │   └──────────────────────────────────────────────────────┘   │  │
│  └──────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────── ─────┘
```

## Prerequisites

### Cluster Side
- A running Kubernetes cluster with Rook-Ceph deployed
- Ceph v20+ (NVMe-oF gateway support)
- The CSI operator enabled (`ROOK_USE_CSI_OPERATOR: "true"` in the Rook operator config)

### External Client
- Linux with kernel 5.0+ (NVMe-oF/TCP support)
- `nvme-cli` installed
- `nvme-tcp` kernel module available
- Network access to the cluster's LoadBalancer IP

---

## Step 1: Create the NVMe-oF Pool

Create a CephBlockPool that the NVMe-oF gateway will use to store volume data.

```yaml
# nvmeof-pool.yaml
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

```bash
kubectl apply -f nvmeof-pool.yaml
```

---

## Step 2: Deploy the NVMe-oF Gateway

Deploy the NVMe-oF gateway that serves volumes over the NVMe-oF/TCP protocol.

```yaml
# nvmeof-gateway.yaml
apiVersion: ceph.rook.io/v1
kind: CephNVMeOFGateway
metadata:
  name: nvmeof
  namespace: rook-ceph
spec:
  image: quay.io/ceph/nvmeof:1.5
  pool: nvmeof
  group: group-a
  instances: 1
  hostNetwork: false
```

```bash
kubectl apply -f nvmeof-gateway.yaml
```

Verify the gateway pod is running:

```bash
kubectl get pods -n rook-ceph | grep nvmeof
```

Expected output:

```
rook-ceph-nvmeof-nvmeof-a-54c5cfd4ff-xxxxx   1/1   Running   0   1m
```

---

## Step 3: Deploy the NVMe-oF CSI Driver

When using the CSI operator (`ROOK_USE_CSI_OPERATOR: "true"`), apply the NVMe-oF CSI
driver resources. This deploys the controller plugin and node plugin DaemonSet.

```yaml
# csi-operator-nvmeof.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ceph-csi-nvmeof-ctrlplugin-sa
  namespace: rook-ceph
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ceph-csi-nvmeof-nodeplugin-sa
  namespace: rook-ceph
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ceph-csi-nvmeof-ctrlplugin-cr
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "patch", "update"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments/status"]
    verbs: ["patch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["csinodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["get", "list", "watch", "patch", "update"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents/status"]
    verbs: ["update", "patch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["serviceaccounts"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["serviceaccounts/token"]
    verbs: ["create"]
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]
  - apiGroups: ["authorization.k8s.io"]
    resources: ["subjectaccessreviews"]
    verbs: ["create"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattributesclasses"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ceph-csi-nvmeof-nodeplugin-cr
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["serviceaccounts"]
    verbs: ["get"]
  - apiGroups: [""]
    resources: ["serviceaccounts/token"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get"]
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ceph-csi-nvmeof-ctrlplugin-crb
subjects:
  - kind: ServiceAccount
    name: ceph-csi-nvmeof-ctrlplugin-sa
    namespace: rook-ceph
roleRef:
  kind: ClusterRole
  name: ceph-csi-nvmeof-ctrlplugin-cr
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ceph-csi-nvmeof-nodeplugin-crb
subjects:
  - kind: ServiceAccount
    name: ceph-csi-nvmeof-nodeplugin-sa
    namespace: rook-ceph
roleRef:
  kind: ClusterRole
  name: ceph-csi-nvmeof-nodeplugin-cr
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ceph-csi-nvmeof-ctrlplugin-r
  namespace: rook-ceph
rules:
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  - apiGroups: ["csiaddons.openshift.io"]
    resources: ["csiaddonsnodes"]
    verbs: ["get", "watch", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["deployments/finalizers", "daemonsets/finalizers"]
    verbs: ["update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ceph-csi-nvmeof-nodeplugin-r
  namespace: rook-ceph
rules:
  - apiGroups: ["csiaddons.openshift.io"]
    resources: ["csiaddonsnodes"]
    verbs: ["get", "watch", "list", "create", "update", "delete"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["deployments/finalizers", "daemonsets/finalizers"]
    verbs: ["update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ceph-csi-nvmeof-ctrlplugin-rb
  namespace: rook-ceph
subjects:
  - kind: ServiceAccount
    name: ceph-csi-nvmeof-ctrlplugin-sa
    namespace: rook-ceph
roleRef:
  kind: Role
  name: ceph-csi-nvmeof-ctrlplugin-r
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ceph-csi-nvmeof-nodeplugin-rb
  namespace: rook-ceph
subjects:
  - kind: ServiceAccount
    name: ceph-csi-nvmeof-nodeplugin-sa
    namespace: rook-ceph
roleRef:
  kind: Role
  name: ceph-csi-nvmeof-nodeplugin-r
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: csi.ceph.io/v1
kind: Driver
metadata:
  name: rook-ceph.nvmeof.csi.ceph.com
  namespace: rook-ceph
spec:
  clusterName: rook-ceph
  controllerPlugin:
    affinity:
      nodeAffinity: {}
    imagePullPolicy: ""
    priorityClassName: system-cluster-critical
    replicas: 1
    resources: {}
  deployCsiAddons: false
  enableMetadata: true
  fsGroupPolicy: File
  generateOMapInfo: false
  imageSet:
    name: rook-csi-operator-image-set-configmap
  log:
    verbosity: 5
  nodePlugin:
    affinity:
      nodeAffinity: {}
    enableSeLinuxHostMount: false
    imagePullPolicy: ""
    kubeletDirPath: /var/lib/kubelet
    priorityClassName: system-node-critical
    resources: {}
    updateStrategy:
      type: RollingUpdate
```

```bash
kubectl apply -f csi-operator-nvmeof.yaml
```

Verify all CSI pods are running:

```bash
kubectl get pods -n rook-ceph | grep nvmeof
```

Expected output:

```
rook-ceph-nvmeof-nvmeof-a-54c5cfd4ff-xxxxx                        1/1   Running   0   2m
rook-ceph.nvmeof.csi.ceph.com-ctrlplugin-8674656c-xxxxx           5/5   Running   0   1m
rook-ceph.nvmeof.csi.ceph.com-nodeplugin-xxxxx                    2/2   Running   0   1m
```

---

## Step 4: Create the StorageClass

The StorageClass tells the CSI driver how to provision NVMe-oF volumes and which
gateway to use.

```yaml
# storageclass.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-nvmeof
parameters:
  clusterID: rook-ceph
  pool: nvmeof
  subsystemNQN: nqn.2016-06.io.spdk:cnode1.rook-ceph
  # Management API - talks to gateway to create subsystems/namespaces
  nvmeofGatewayAddress: "rook-ceph-nvmeof-nvmeof-a"
  nvmeofGatewayPort: "5500"
  # Data Plane - worker nodes connect here for actual I/O
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
  csi.storage.k8s.io/node-expand-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-expand-secret-namespace: rook-ceph
  imageFormat: "2"
  imageFeatures: layering,deep-flatten,exclusive-lock,object-map,fast-diff
# When using the CSI operator (ROOK_USE_CSI_OPERATOR=true), the driver name
# is prefixed with the operator namespace: rook-ceph.nvmeof.csi.ceph.com
# Without the CSI operator, use: nvmeof.csi.ceph.com
provisioner: rook-ceph.nvmeof.csi.ceph.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
```

```bash
kubectl apply -f storageclass.yaml
```

---

## Step 5: Create a PVC (Provision the Volume)

This PVC triggers the CSI driver to create an RBD image and register it as an
NVMe namespace in the gateway.

```yaml
# pvc.yaml
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

```bash
kubectl apply -f pvc.yaml
```

Verify the PVC is bound:

```bash
kubectl get pvc nvmeof-external-volume
```

Expected output:

```
NAME                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS
nvmeof-external-volume   Bound    pvc-73077310-c0b0-4890-9500-2bc5551e2f12   128Mi      RWO            ceph-nvmeof
```

---

## Step 6: Expose the NVMe-oF Gateway Externally

The gateway service is ClusterIP by default (internal only). To allow external clients
to connect, create a LoadBalancer service that exposes the NVMe-oF ports.

### Gateway Ports

| Port | Purpose                          |
|------|----------------------------------|
| 4420 | NVMe-oF data transport (I/O)     |
| 5500 | gRPC management API              |
| 8009 | NVMe-oF discovery service        |

### Create the LoadBalancer Service

> **Security Note:** Do not expose port 5500 (gRPC management API) externally.
> It is only used for internal cluster communication between the CSI driver and
> the gateway.

```yaml
# nvmeof-loadbalancer.yaml
apiVersion: v1
kind: Service
metadata:
  name: rook-ceph-nvmeof-nvmeof-a-lb
  namespace: rook-ceph
spec:
  type: LoadBalancer
  selector:
    app: rook-ceph-nvmeof
    ceph_daemon_id: nvmeof-a
    ceph_daemon_type: nvmeof
    rook_cluster: rook-ceph
  ports:
    - name: io
      port: 4420
      targetPort: 4420
      protocol: TCP
    - name: discovery
      port: 8009
      targetPort: 8009
      protocol: TCP
```

```bash
kubectl apply -f nvmeof-loadbalancer.yaml
```

Wait for the external IP/hostname to be assigned:

```bash
kubectl get svc -n rook-ceph rook-ceph-nvmeof-nvmeof-a-lb -w
```

Expected output (IBM Cloud example):

```
NAME                           TYPE           CLUSTER-IP       EXTERNAL-IP                           PORT(S)
rook-ceph-nvmeof-nvmeof-a-lb   LoadBalancer   172.30.149.173   fa38eb00-us-east.lb.appdomain.cloud   4420:30601/TCP,8009:30144/TCP
```

> **Note:** On IBM Cloud, the LoadBalancer assigns a DNS hostname. DNS propagation
> can take 5-10 minutes. Verify from your external machine with `host <hostname>` before proceeding.

```bash
$ host fa38eb00-us-east.lb.appdomain.cloud
fa38eb00-us-east.lb.appdomain.cloud has address 169.62.19.103
fa38eb00-us-east.lb.appdomain.cloud has address 150.240.2.147
```

---

## Step 7: Add the External Client as an Allowed Host

By default, the NVMe-oF subsystem uses host-based access control (`allow_any_host: false`),
which rejects connections from unknown clients. To allow any external client to connect,
set `allow_any_host: true` by adding the wildcard host (`*`) via the gateway gRPC API.

### 7a. Get the Gateway Pod IP

```bash
kubectl get pod -n rook-ceph -l ceph_daemon_type=nvmeof -o jsonpath='{.items[0].status.podIP}'
```

### 7b. Allow Any Host to Connect

```bash
kubectl -n rook-ceph exec deploy/rook-ceph-nvmeof-nvmeof-a -- python3 -c "
import sys; sys.path.insert(0, '/src')
from control.proto import gateway_pb2 as pb2, gateway_pb2_grpc as pb2_grpc
import grpc

pod_ip = '$(kubectl get pod -n rook-ceph -l ceph_daemon_type=nvmeof -o jsonpath='{.items[0].status.podIP}')'
channel = grpc.insecure_channel(pod_ip + ':5500')
stub = pb2_grpc.GatewayStub(channel)

nqn = 'nqn.2016-06.io.spdk:cnode1.rook-ceph'

req = pb2.add_host_req(subsystem_nqn=nqn, host_nqn='*')
resp = stub.add_host(req)
print('Allow any host result:', resp.error_message)

# Verify
resp2 = stub.list_subsystems(pb2.list_subsystems_req())
for s in resp2.subsystems:
    if s.nqn == nqn:
        print('allow_any_host:', s.allow_any_host)
"
```

Expected output:

```
Allow any host result: Success
allow_any_host: True
```

> **Note:** Using `host_nqn='*'` sets `allow_any_host: true` on the subsystem, allowing
> any external client to connect without registering its NQN. For production environments,
> you may want to add specific client NQNs instead (replace `'*'` with the client's NQN
> from `cat /etc/nvme/hostnqn` on the external machine).

---

## Step 8: Connect from the External Client

### 8a. Load the NVMe-oF/TCP Kernel Module

```bash
sudo modprobe nvme-tcp
```

Verify it's loaded:

```bash
lsmod | grep nvme_tcp
```

Expected output:

```
nvme_tcp              102400  0
nvme_fabrics           49152  1 nvme_tcp
nvme_core             274432  6 nvme_tcp,nvme,nvme_fabrics
```

> **Tip:** To load automatically on boot, add `nvme-tcp` to `/etc/modules-load.d/nvme-tcp.conf`.

### 8b. Connect to the NVMe-oF Subsystem

Connect directly using the subsystem NQN and LoadBalancer address:

```bash
sudo nvme connect -t tcp \
  -n nqn.2016-06.io.spdk:cnode1.rook-ceph \
  -a <LOADBALANCER_IP_OR_HOSTNAME> \
  -s 4420
```

**Example:**

```bash
sudo nvme connect -t tcp \
  -n nqn.2016-06.io.spdk:cnode1.rook-ceph \
  -a fa38eb00-us-east.lb.appdomain.cloud \
  -s 4420
```

Expected output:

```
connecting to device: nvme2
```

### 8c. Verify the Connection

```bash
sudo nvme list-subsys
```

Expected output:

```
nvme-subsys2 - NQN=nqn.2016-06.io.spdk:cnode1.rook-ceph
               hostnqn=nqn.2014-08.org.nvmexpress:uuid:f8cf014c-22cd-11b2-a85c-ea3a6f188b53
\
 +- nvme2 tcp traddr=169.62.19.103,trsvcid=4420,src_addr=192.168.1.201 live
```

The connection status should show **live**.

### 8d. Check the Block Device

```bash
sudo nvme list
```

```
Node          SN                  Model                     Namespace  Usage              Format
/dev/nvme2n1  Ceph7419772823850   Ceph bdev Controller      0x1        134.22 MB / 134.22 MB  4 KiB + 0 B
```

```bash
lsblk | grep nvme
```

```
nvme2n1   259:5   0   128M   0   disk
```

---

## Step 9: Format and Mount the Volume

### First-time use (format the device)

```bash
sudo mkfs.ext4 /dev/nvme2n1
```

### Mount the device

```bash
sudo mkdir -p /mnt/nvmeof
sudo mount /dev/nvme2n1 /mnt/nvmeof
```

### Set ownership (optional, for non-root access)

```bash
sudo chown $(whoami):$(whoami) /mnt/nvmeof
```

### Verify

```bash
df -h /mnt/nvmeof
```

```
Filesystem      Size  Used Avail Use% Mounted on
/dev/nvme2n1    104M  152K   95M   1% /mnt/nvmeof
```

### Read and write data

```bash
echo "Hello from external client!" > /mnt/nvmeof/hello.txt
cat /mnt/nvmeof/hello.txt
```

### Real-world test output from external machine

```bash
machine/mnt/nvmeof$ df -h /mnt/nvmeof/
Filesystem      Size  Used Avail Use% Mounted on
/dev/nvme2n1    104M  168K   95M   1% /mnt/nvmeof

machine/mnt/nvmeof$ echo "abcd" > new.txt

machine/mnt/nvmeof$ cat new.txt
abcd

machine/mnt/nvmeof$ ls /mnt/nvmeof/
lost+found  new.txt  test.txt
```

---

## Step 10: Verify Data from Inside the Cluster (Optional)

You can verify the data written from the external client is visible inside the cluster
using a privileged debug pod.

### Create a debug pod

```yaml
# nvme-debug-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: nvme-client-test
  namespace: rook-ceph
spec:
  containers:
    - name: nvme-client
      image: fedora:latest
      command: ["/bin/bash", "-c", "dnf install -y nvme-cli e2fsprogs && sleep 3600"]
      securityContext:
        privileged: true
  restartPolicy: Never
  nodeSelector:
    node-role.kubernetes.io/worker: ""
```

```bash
kubectl apply -f nvme-debug-pod.yaml
```

Wait for the pod to be ready (nvme-cli needs to install):

```bash
kubectl wait --for=condition=Ready pod/nvme-client-test -n rook-ceph --timeout=120s
sleep 30  # wait for dnf install to finish
```

### Add the worker node's NQN to allowed hosts

The debug pod runs on a worker node. You need to add that node's NQN to the
subsystem's allowed hosts (same gRPC method as Step 7b).

Get the worker node's NQN:

```bash
kubectl exec -n rook-ceph nvme-client-test -- nvme show-hostnqn
```

Then add it using the gRPC API (Step 7b).

### Unmount the volume on the external client

On the external client, unmount and disconnect before mounting inside the pod:

```bash
sudo umount /mnt/nvmeof
sudo nvme disconnect -n nqn.2016-06.io.spdk:cnode1.rook-ceph
```

### Mount the volume inside the pod

Once the external client has been fully disconnected, mount the volume in the
debug pod:

```bash
kubectl exec -n rook-ceph nvme-client-test -- bash -c \
  "mkdir -p /mnt/nvmeof && \
   mount /dev/nvme0n1 /mnt/nvmeof && \
   ls -la /mnt/nvmeof/ && \
   cat /mnt/nvmeof/hello.txt"
```

### Real-world test: verifying data from inside the pod

After writing `new.txt` on the external PC, unmounting, and disconnecting,
exec into the debug pod to confirm the data is visible:

```bash
$ kubectl exec -it -n rook-ceph nvme-client-test -- /bin/bash
sh-5.3# ls /mnt/nvmeof/
lost+found  new.txt  test.txt

sh-5.3# cat /mnt/nvmeof/new.txt
abcd

sh-5.3# cat /mnt/nvmeof/test.txt
Hello from NVMe-oF over Ceph on IBM Cloud!
```

The files written from the external PC (`new.txt`) are visible inside the
Kubernetes cluster pod, confirming end-to-end NVMe-oF connectivity:

```
 1. External PC writes           2. Pod reads (after disconnect)
 ──────────────────               ──────────────────────────────
 /mnt/nvmeof/                    /mnt/nvmeof/
   ├── new.txt  ── write ──┐        ┌── read ──  new.txt
   ├── test.txt             │        │            test.txt
   └── lost+found/          ▼        ▼            lost+found/
                     ┌──────────────┐
                     │  Ceph RBD    │
                     │  (nvmeof     │
                     │   pool)      │
                     └──────────────┘
                  Same storage backend
```

---

## Cleanup

### On the external client

```bash
# Unmount the volume
sudo umount /mnt/nvmeof

# Disconnect from the NVMe-oF subsystem
sudo nvme disconnect -n nqn.2016-06.io.spdk:cnode1.rook-ceph

# Verify disconnected
sudo nvme list-subsys
```

### On the cluster

```bash
# Delete the debug pod
kubectl delete pod -n rook-ceph nvme-client-test

# Delete the LoadBalancer service
kubectl delete svc -n rook-ceph rook-ceph-nvmeof-nvmeof-a-lb

# Delete the PVC (this also removes the RBD image and NVMe namespace)
kubectl delete pvc nvmeof-external-volume

# (Optional) Delete the test pod if it was created
kubectl delete pod nvmeof-test-pod
```

---

## Troubleshooting

### Connection refused or timeout

- Verify the LoadBalancer DNS has resolved: `host <lb-hostname>`
- Check the gateway pod is running: `kubectl get pods -n rook-ceph | grep nvmeof`
- Verify `nvme-tcp` module is loaded: `lsmod | grep nvme_tcp`
- Check firewall rules allow TCP port 4420 outbound

### "Subsystem does not allow host" in gateway logs

The client's NQN is not in the subsystem's allowed hosts list. Add it using the
gRPC API (Step 7b). Check gateway logs for the rejected NQN:

```bash
kubectl logs -n rook-ceph deploy/rook-ceph-nvmeof-nvmeof-a --tail=50
```

Look for messages like:

```
Subsystem 'nqn.2016-06.io.spdk:cnode1.rook-ceph' does not allow host 'nqn.2014-08...'
```

### `kubectl port-forward` does not work for NVMe-oF

Kubernetes port-forward tunnels TCP through HTTP/WebSocket, which is incompatible
with the NVMe-oF/TCP protocol. You must use direct TCP access via LoadBalancer,
NodePort (with VPN/direct network access), or connect from within the cluster.

### Discovery returns 0 records

This is normal when no hosts are in the allowed list. The subsystem uses per-host
access control. Add the client's NQN first (Step 7), then discovery will return
the subsystem entry. You can also skip discovery and connect directly using the
subsystem NQN (Step 8b).
