---
title: Toolbox
weight: 60
---

#  Rook Toolbox
The Rook toolbox is a container with common tools used for rook debugging and testing.
The toolbox is based on Ubuntu, so more tools of your choosing can be easily installed with `apt-get`. 

## Running the Toolbox in Kubernetes

The rook toolbox can run as a pod in a Kubernetes cluster.  After you ensure you have a running Kubernetes cluster with rook deployed (see the [Kubernetes](kubernetes.md) instructions),
launch the rook-tools pod.

Save the tools spec as `rook-tools.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: rook-tools
  namespace: rook
spec:
  containers:
  - name: rook-tools
    image: rook/toolbox:master
    imagePullPolicy: IfNotPresent
    args: ["sleep", "36500d"]
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

## Running the Toolbox for Standalone

### Container Linux by CoreOS

To use the rook toolbox on CoreOS, first add the following values to the toolbox config file:
```bash
cat >~/.toolboxrc <<EOL
TOOLBOX_DOCKER_IMAGE=rook/toolbox
TOOLBOX_DOCKER_TAG=latest
EOL
```

Then launch the toolbox as usual:
```bash
toolbox
```

#### Ceph Tools

To use the ceph tools from a rook host, launch the toolbox with the following options:
```bash
toolbox --bind=/var/lib/rook:/var/lib/rook /toolbox/entrypoint.sh
```
Then you can run `ceph` and `rados` commands like usual:
```bash
ceph df
rados df
```

### Other Linux Distros

The rook toolbox container can simply be run directly with `docker` on other Linux distros:
```bash
docker run -it rook/toolbox
```

#### Ceph Tools

To run ceph tools such as `ceph` and `rados`, run the container with the following options:
```bash
docker run -it --network=host -v /var/lib/rook:/var/lib/rook rook/toolbox
```
