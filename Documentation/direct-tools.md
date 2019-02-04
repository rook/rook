---
title: Direct Tools
weight: 11200
indent: true
---

# Direct Tools

Rook is designed with Kubernetes design principles from the ground up. This topic is going to escape the bounds of Kubernetes storage and show you how to
use block and file storage directly from a pod without any of the Kubernetes magic. The purpose of this topic is to help you quickly test a new configuration,
although it is not meant to be used in production. All of the benefits of Kubernetes storage including failover, detach, and attach will not be available.
If your pod dies, your mount will die with it.

## Block Storage Tools

After you have created a pool as described in the [Block Storage](ceph-block.md) topic, you can create a block image and mount it directly in a pod.
This example will show how the Ceph rbd volume can be mounted in the toolbox pod.

After you have started and connected to the [Rook toolbox](ceph-toolbox.md), proceed with the following commands in the toolbox.

Create a volume image (10MB):
```bash
rbd create replicapool/test --size 10
rbd info replicapool/test

# Disable the rbd features that are not in the kernel module
rbd feature disable replicapool/test fast-diff deep-flatten object-map
```

Map the block volume and format it and mount it:
```bash
# Map the rbd device. If the toolbox was started with "hostNetwork: false" this hangs and you have to stop it with Ctrl-C,
# however the command still succeeds; see https://github.com/rook/rook/issues/2021
rbd map replicapool/test

# Find the device name, such as rbd0
lsblk | grep rbd

# Format the volume (only do this the first time or you will lose data)
mkfs.ext4 -m0 /dev/rbd0

# Mount the block device
mkdir /tmp/rook-volume
mount /dev/rbd0 /tmp/rook-volume
```

Write and read a file:
```bash
echo "Hello Rook" > /tmp/rook-volume/hello
cat /tmp/rook-volume/hello
```

### Unmount the Block device

Unmount the volume and unmap the kernel device:
```bash
umount /tmp/rook-volume
rbd unmap /dev/rbd0
```

## Shared Filesystem Tools

After you have created a file system as described in the [Shared Filesystem](ceph-filesystem.md) topic, you can mount the filesystem from multiple pods.
The the other topic you may have mounted the filesystem already in the registry pod. Now we will mount the same file system in the toolbox pod.
This is just a simple way to validate the Ceph file system and is not recommended for production Kubernetes pods.

After you have started and connected to the [Rook toolbox](ceph-toolbox.md), proceed with the following commands in the toolbox.

```bash
# Create the directory
mkdir /tmp/registry

# Detect the mon endpoints and the user secret for the connection
mon_endpoints=$(grep mon_host /etc/ceph/ceph.conf | awk '{print $3}')
my_secret=$(grep key /etc/ceph/keyring | awk '{print $3}')

# Mount the file system
mount -t ceph -o mds_namespace=myfs,name=admin,secret=$my_secret $mon_endpoints:/ /tmp/registry

# See your mounted file system
df -h
```

Now you should have a mounted file system. If you have pushed images to the registry you will see a directory called `docker`.
```
ls /tmp/registry
```

Try writing and reading a file to the shared file system.

```bash
echo "Hello Rook" > /tmp/registry/hello
cat /tmp/registry/hello

# delete the file when you're done
rm -f /tmp/registry/hello
```

### Unmount the Filesystem

To unmount the shared file system from the toolbox pod:
```bash
umount /tmp/registry
rmdir /tmp/registry
```

No data will be deleted by unmounting the file system.
