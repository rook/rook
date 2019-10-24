---
title: Toolbox
weight: 11100
indent: true
---

# Rook Toolbox

The Rook toolbox is a container with common tools used for rook debugging and testing.
The toolbox is based on CentOS, so more tools of your choosing can be easily installed with `yum`.

## Running the Toolbox in Kubernetes

The rook toolbox can run as a deployment in a Kubernetes cluster.  After you ensure you have a running Kubernetes cluster with rook deployed (see the [Kubernetes](ceph-quickstart.md) instructions),
launch the rook-ceph-tools pod.

Save the tools spec as `toolbox.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-ceph-tools
  namespace: rook-ceph
  labels:
    app: rook-ceph-tools
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rook-ceph-tools
  template:
    metadata:
      labels:
        app: rook-ceph-tools
    spec:
      dnsPolicy: ClusterFirstWithHostNet
      containers:
      - name: rook-ceph-tools
        image: rook/ceph:master
        command: ["/tini"]
        args: ["-g", "--", "/usr/local/bin/toolbox.sh"]
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
      # if hostNetwork: false, the "rbd map" command hangs, see https://github.com/rook/rook/issues/2021
      hostNetwork: true
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

Launch the rook-ceph-tools pod:

```console
kubectl create -f toolbox.yaml
```

Wait for the toolbox pod to download its container and get to the `running` state:

```console
kubectl -n rook-ceph get pod -l "app=rook-ceph-tools"
```

Once the rook-ceph-tools pod is running, you can connect to it with:

```console
kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}') bash
```

All available tools in the toolbox are ready for your troubleshooting needs. 

**Example**:

* `ceph status`
* `ceph osd status`
* `ceph df`
* `rados df`

When you are done with the toolbox, you can remove the deployment:

```console
kubectl -n rook-ceph delete deployment rook-ceph-tools
```

## Troubleshooting without the Toolbox

The Ceph tools will commonly be the only tools needed to troubleshoot a cluster. In that case, you can connect to any of the rook pods and execute the ceph commands in the same way that you would in the toolbox pod such as the mon pods or the operator pod.
If connecting to the mon pods, make sure you connect to the mon most recently started. The mons keep the config updated in memory after starting and may not have the latest config on disk.
For example, after starting the cluster connect to the `mon2` pod instead of `mon0`.
