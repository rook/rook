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

This guide assumes a Rook cluster as explained in the [Quickstart](../../Getting-Started/quickstart.md).

### Requirements

- **Ceph Version**: Ceph v20 (Tentacle) or later with NVMe-oF gateway support
- **Rook Version**: Rook v1.19.0 or later
- **Ceph CSI Driver**: NVMe-oF-enabled CSI driver (nvmeof.csi.ceph.com)
- **ServiceAccount**: The `ceph-nvmeof-gateway` ServiceAccount must exist in the namespace where the gateway will be deployed

!!! note
    For Ceph v20 and above, the NVMe-oF gateway feature is available. Ensure your Ceph cluster is running a compatible version before proceeding.

## Architecture Overview

The NVMe-oF implementation separates concerns between infrastructure management and storage provisioning:

- **Rook Operator**: Manages NVMe-oF gateway pod lifecycle, scaling, and health monitoring
- **Ceph CSI Driver**: Handles dynamic provisioning, subsystem creation, and NVMe namespace management
- **NVMe-oF Gateways**: Serve NVMe-oF protocol and manage RBD backend connections

When a PVC is created using the NVMe-oF storage class:
1. The CSI driver creates an RBD image in the specified pool
2. The CSI driver creates an NVMe subsystem and namespace on the gateway
3. The volume can be accessed either:
   - By Kubernetes pods within the cluster via NVMe-oF protocol
   - By external clients outside the cluster using standard NVMe-oF initiators

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

### Create ServiceAccount and RBAC

First, create the ServiceAccount and required RBAC permissions:

```yaml
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: nvmeof-csi-provisioner
  namespace: rook-ceph
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: nvmeof-external-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch", "update", "patch", "create"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots/status"]
    verbs: ["get", "list", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments/status"]
    verbs: ["patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["csinodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: nvmeof-csi-provisioner-role
subjects:
  - kind: ServiceAccount
    name: nvmeof-csi-provisioner
    namespace: rook-ceph
roleRef:
  kind: ClusterRole
  name: nvmeof-external-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: rook-ceph
  name: nvmeof-external-provisioner-cfg
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: nvmeof-csi-provisioner-role-cfg
  namespace: rook-ceph
subjects:
  - kind: ServiceAccount
    name: nvmeof-csi-provisioner
    namespace: rook-ceph
roleRef:
  kind: Role
  name: nvmeof-external-provisioner-cfg
  apiGroup: rbac.authorization.k8s.io
```

Apply the RBAC configuration:

```console
kubectl create -f nvmeof-csi-rbac.yaml
```

### Deploy the CSI Provisioner

Deploy the CSI provisioner with the NVMe-oF driver:

```yaml
---
kind: Deployment
apiVersion: apps/v1
metadata:
  name: csi-nvmeofplugin-provisioner
  namespace: rook-ceph
spec:
  replicas: 1
  selector:
    matchLabels:
      app: csi-nvmeofplugin-provisioner
  template:
    metadata:
      labels:
        app: csi-nvmeofplugin-provisioner
    spec:
      serviceAccountName: nvmeof-csi-provisioner
      priorityClassName: system-cluster-critical
      containers:
        - name: csi-nvmeofplugin
          image: quay.io/ceph/cephcsi:canary  # Use appropriate version
          command: ["/usr/local/bin/cephcsi"]
          args:
            - "--nodeid=$(NODE_ID)"
            - "--type=nvmeof"
            - "--controllerserver=true"
            - "--endpoint=$(CSI_ENDPOINT)"
            - "--v=5"
            - "--drivername=nvmeof.csi.ceph.com"
            - "--pidlimit=-1"
            - "--rbdhardmaxclonedepth=8"
            - "--rbdsoftmaxclonedepth=4"
            - "--enableprofiling=false"
            - "--setmetadata=true"
          env:
            - name: POD_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
            - name: NODE_ID
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: CSI_ENDPOINT
              value: unix:///csi/csi-provisioner.sock
          imagePullPolicy: "IfNotPresent"
          securityContext:
            privileged: true
            capabilities:
              drop:
                - ALL
          volumeMounts:
            - name: socket-dir
              mountPath: /csi
            - name: ceph-csi-config
              mountPath: /etc/ceph-csi-config/
            - name: keys-tmp-dir
              mountPath: /tmp/csi/keys
        - name: csi-provisioner
          image: registry.k8s.io/sig-storage/csi-provisioner:v5.1.0
          args:
            - "--csi-address=$(ADDRESS)"
            - "--v=1"
            - "--timeout=150s"
            - "--retry-interval-start=500ms"
            - "--leader-election=true"
            - "--feature-gates=HonorPVReclaimPolicy=true"
            - "--prevent-volume-mode-conversion=true"
            - "--default-fstype=ext4"
            - "--extra-create-metadata=true"
            - "--immediate-topology=false"
            - "--http-endpoint=$(POD_IP):8090"
          env:
            - name: ADDRESS
              value: unix:///csi/csi-provisioner.sock
            - name: POD_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
          imagePullPolicy: "IfNotPresent"
          ports:
            - containerPort: 8090
              name: provisioner
              protocol: TCP
          volumeMounts:
            - name: socket-dir
              mountPath: /csi
        - name: csi-attacher
          image: registry.k8s.io/sig-storage/csi-attacher:v4.8.0
          args:
            - "--v=1"
            - "--csi-address=$(ADDRESS)"
            - "--leader-election=true"
            - "--retry-interval-start=500ms"
            - "--default-fstype=ext4"
            - "--http-endpoint=$(POD_IP):8093"
          env:
            - name: ADDRESS
              value: /csi/csi-provisioner.sock
            - name: POD_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
          imagePullPolicy: "IfNotPresent"
          ports:
            - containerPort: 8093
              name: attacher
              protocol: TCP
          volumeMounts:
            - name: socket-dir
              mountPath: /csi
        - name: csi-resizer
          image: registry.k8s.io/sig-storage/csi-resizer:v1.12.0
          args:
            - "--csi-address=$(ADDRESS)"
            - "--v=5"
            - "--leader-election=true"
            - "--handle-volume-inuse-error=false"
            - "--feature-gates=VolumeAttributesClass=true"
          env:
            - name: ADDRESS
              value: unix:///csi/csi-provisioner.sock
          volumeMounts:
            - name: socket-dir
              mountPath: /csi
      volumes:
        - name: socket-dir
          emptyDir:
            medium: "Memory"
        # Mount rook-ceph-csi-config ConfigMap
        # This ConfigMap contains the cluster configuration with monitor endpoints
        # that Ceph-CSI needs to connect to the Ceph cluster
        - name: ceph-csi-config
          configMap:
            name: rook-ceph-csi-config
            items:
              - key: csi-cluster-config-json
                path: config.json
        - name: keys-tmp-dir
          emptyDir:
            medium: "Memory"
```

!!! important
    The `ceph-csi-config` volume must mount the `rook-ceph-csi-config` ConfigMap, which contains the Ceph cluster configuration. This ConfigMap is automatically created by the Rook operator.

Apply the CSI provisioner:

```console
kubectl create -f csi-nvmeofplugin-provisioner.yaml
```

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
