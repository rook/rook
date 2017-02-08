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
To use the ceph tools from a rook host, launch the toolbox and run the following command:
```
toolbox --bind=/var/lib/rook:/var/lib/rook
ROOK_CONFIG=`find /var/lib/rook -regex '.*mon[0-9]+/.*\.config' | head -1`
```
Then you can run `ceph` and `rados` commands like usual:
```
ceph -c ${ROOK_CONFIG} df
rados -c ${ROOK_CONFIG} df
```