#  Rook Toolbox
The rook toolbox is a container with common tools used for rook debugging and testing.  All packages in the toolbox can be seen in the [Dockerfile](/toolbox/Dockerfile)

## Installing more tools
The rook toolbox is based on Ubuntu, so more tools of your choosing can be easily installed with `apt-get`.  For example, to install `telnet`:
```bash
apt-get update
apt-get install telnet
```

## Running the Toolbox in Kubernetes

The rook toolbox can run as a pod in a Kubernetes cluster.  First, ensure you have a running Kubernetes cluster with rook deployed (see the [Kubernetes](kubernetes.md) instructions).

Launch the rook-tools pod:
```bash
cd demo/kubernetes
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

When you are done with the toolbox, you can clean it up by running:
```bash
kubectl delete -f rook-tools.yaml
```

## Running the Toolbox for Standalone

### Container Linux by CoreOS
To use the rook toolbox on CoreOS, first add the following values to the toolbox config file:
```bash
cat >~/.toolboxrc <<EOL
TOOLBOX_DOCKER_IMAGE=quay.io/rook/toolbox
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
docker run -it quay.io/rook/toolbox
```

#### Ceph Tools
To run ceph tools such as `ceph` and `rados`, run the container with the following options:
```bash
docker run -it --network=host -v /var/lib/rook:/var/lib/rook quay.io/rook/toolbox
```
