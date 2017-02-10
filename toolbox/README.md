#  Rook Toolbox
The rook toolbox is a container with common tools used for rook debugging and testing.  All packages in the toolbox can be seen in the [Dockerfile](Dockerfile)

## Container Linux by CoreOS
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

### Ceph Tools
To use the ceph tools from a rook host, launch the toolbox with the following options:
```
toolbox --bind=/var/lib/rook:/var/lib/rook /toolbox/entrypoint.sh
```
Then you can run `ceph` and `rados` commands like usual:
```
ceph df
rados df
```

## Other Linux Distros
The rook toolbox container can simply be run directly with `docker` on other Linux distros:
```
docker run -it quay.io/rook/toolbox
```

### Ceph Tools
To run ceph tools such as `ceph` and `rados`, run the container with the following options:
```
docker run -it --network=host -v /var/lib/rook:/var/lib/rook quay.io/rook/toolbox
```