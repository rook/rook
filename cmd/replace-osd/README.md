# replace-osd

replace-osd is a program to replace an OSD with new one.

# prerequisite

- `toolbox` pod should be deployed.
- Only works on PVC-based cluster.

## How to build

```console
make
```

## How to use

```console
# replace osd 2 with a new one.
./replace-osd -ns rook-ceph -osd 2
```

For more information, please run `./replace-osd -help`
