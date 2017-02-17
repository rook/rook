#  Rook Toolbox
The rook toolbox is a container with common tools used for rook debugging and testing.  All packages in the toolbox can be seen in the [Dockerfile](Dockerfile)

## Installing more tools
The rook toolbox is based on Ubuntu, so more tools of your choosing can be easily installed with `apt-get`.  For example, to install `telnet`:
```
apt-get update
apt-get install telnet
```

## Running the toolbox
### Kubernetes
The rook toolbox can run as a pod in a Kubernetes cluster.  First, ensure you have a running Kubernetes cluster with rook deployed (instructions can be found in the [Rook on Kubernetes readme](../demo/kubernetes/README.md)).

From this directory, launch the rook-tools pod:
```
kubectl create -f rook-tools.yml
```

Wait for the toolbox pod to download its container and get to the `running` state:
```
kubectl -n rook get pod rook-tools
```

Once the rook-tools pod is running, you can connect to it with:
```
kubectl -n rook exec -it rook-tools bash
```

All available tools in the toolbox are ready for your troubleshooting needs.  Example:
```
rook status
ceph df
rados df
```

When you are completely done with the toolbox, you can clean it up by running:
```
kubectl delete -f rook-tools.yml
```

### Container Linux by CoreOS
To use the rook toolbox on CoreOS, first add the following values to the toolbox config file:
```
cat >~/.toolboxrc <<EOL
TOOLBOX_DOCKER_IMAGE=quay.io/rook/toolbox
TOOLBOX_DOCKER_TAG=latest
EOL
```

Then launch the toolbox as usual:
```
toolbox
```

#### Ceph Tools
To use the ceph tools from a rook host, launch the toolbox with the following options:
```
toolbox --bind=/var/lib/rook:/var/lib/rook /toolbox/entrypoint.sh
```
Then you can run `ceph` and `rados` commands like usual:
```
ceph df
rados df
```

### Other Linux Distros
The rook toolbox container can simply be run directly with `docker` on other Linux distros:
```
docker run -it quay.io/rook/toolbox
```

#### Ceph Tools
To run ceph tools such as `ceph` and `rados`, run the container with the following options:
```
docker run -it --network=host -v /var/lib/rook:/var/lib/rook quay.io/rook/toolbox
```