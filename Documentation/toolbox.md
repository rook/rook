---
title: Toolbox
weight: 72
indent: true
---

#  Rook Toolbox
The Rook toolbox is a container with common tools used for rook debugging and testing.
The toolbox is based on Ubuntu, so more tools of your choosing can be easily installed with `apt-get`. 

## Running the Toolbox in Kubernetes

The rook toolbox can run as a pod in a Kubernetes cluster.  After you ensure you have a running Kubernetes cluster with rook deployed (see the [Kubernetes](quickstart.md) instructions),
launch the rook-tools pod.

Save the tools spec as `rook-tools.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: rook-tools
  namespace: rook
spec:
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: rook-tools
    image: rook/toolbox:master
    imagePullPolicy: IfNotPresent
    env:
      - name: ROOK_ADMIN_SECRET
        valueFrom:
          secretKeyRef:
            name: rook-ceph-mon
            key: admin-secret
    securityContext:
      privileged: true
    volumeMounts:
      - mountPath: /dev
        name: dev
      - mountPath: /sys/bus
        name: sysbus
      - mountPath: /lib/modules
        name: libmodules
      - name: mon-endpoint-volume
        mountPath: /etc/rook
  hostNetwork: false
  volumes:
    - name: dev
      hostPath:
        path: /dev
    - name: sysbus
      hostPath:
        path: /sys/bus
    - name: libmodules
      hostPath:
        path: /lib/modules
    - name: mon-endpoint-volume
      configMap:
        name: rook-ceph-mon-endpoints
        items:
        - key: data
          path: mon-endpoints
```

Launch the rook-tools pod:
```bash
kubectl create -f rook-tools.yaml
```

Wait for the toolbox pod to download its container and get to the `running` state:
```bash
kubectl -n rook get pod rook-tools
```

Once the rook-tools pod is running, you can connect to it with:
```bash
kubectl -n rook exec -it rook-tools bash
```

All available tools in the toolbox are ready for your troubleshooting needs.  Example:
```bash
rookctl status
ceph df
rados df
```

When you are done with the toolbox, remove the pod:
```bash
kubectl -n rook delete pod rook-tools
```
