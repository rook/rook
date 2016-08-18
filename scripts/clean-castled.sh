#!/bin/bash

# clean up mon dirs
rm -fr /tmp/mon*

# clean up bootstrap-osd dir
rm -fr /tmp/bootstrap-osd

# unmount OSD volumes and delete their directories
for dir in `mount | grep -E '/tmp/osd[0-9]+ ' | awk '{print $3}'`; do sudo umount -f ${dir}; done
rm -fr /tmp/osd*

# delete all etcd keys
etcdctl rm --recursive /castle/ceph