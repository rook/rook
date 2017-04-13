# Consume filesystem in Kubernetes

Rook filesystem can be mounted read-write from multiple pods. This may be useful for applications which can be clustered using a shared filesystem. In this example I am running [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

### Prerequisites

This guide assumes you have created rook cluster and pool as explained in [Kubernetes guide](kubernetes.md)

### Create rook filesystem

Assuming you have [rook-tools](toolbox.md) pod running.

```
kubectl -n rook exec rook-tools -it bash
rook filesystem create --name registryFS
```

### Optional: Adjust pool paramaters

In this case I am setting the replication to 2

```
kubectl -n rook exec rook-tools -it bash
ceph osd pool set registryFS-data size 2
ceph osd pool set registryFS-metadata size 2
```

### Optional: Copy admin key to desired namespace

This is required if you are consuming the filesystem from a namespace other than `rook`. In this example we are deploying to `kube-registry`

```
kubectl get secret rook-admin -n rook -o json | jq '.metadata.namespace = "kube-system"' | kubectl apply -f -
```

### Deploy the applications

Example RC:-

```
apiVersion: v1
kind: ReplicationController
metadata:
  name: kube-registry-v0
  namespace: kube-system
  labels:
    k8s-app: kube-registry
    version: v0
    kubernetes.io/cluster-service: "true"
spec:
  replicas: 3
  selector:
    k8s-app: kube-registry
    version: v0
  template:
    metadata:
      labels:
        k8s-app: kube-registry
        version: v0
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: registry
        image: registry:2
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
        env:
        - name: REGISTRY_HTTP_ADDR
          value: :5000
        - name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
          value: /var/lib/registry
        volumeMounts:
        - name: image-store
          mountPath: /var/lib/registry
        ports:
        - containerPort: 5000
          name: registry
          protocol: TCP
      volumes:
      - name: image-store
        cephfs:
          monitors:
          - mon.rook.svc.cluster.local:6790
          user: admin
          secretRef:
            name: rook-admin
```

Replace `mon.rook.svc.cluster.local:6790` with your actual monitor IPs, the same ones used for [StorageClass](kubernetes.md#provision-storage)

You now have a docker registry which is HA with persistent storage.

### Test the storage

We did not mention which rook filesystem to use. It used `registryFS`, probably because that is the only one. We can verify it using :-

```
kubectl -n rook exec rook-tools -it bash
mkdir /tmp/foo
rook filesystem mount --name registryFS --path /tmp/foo
ls /tmp/foo #Here you should see a directory called docker created by the registry
rook filesystem unmount --path /tmp/foo
rmdir /tmp/foo
```
