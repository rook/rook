#!/bin/bash

# clean up mon dirs
rm -fr /tmp/mon*

# clean up bootstrap-osd dir
rm -fr /tmp/bootstrap-osd

# unmount OSD volumes and delete their directories
for dir in `mount | grep -E '/tmp/osd[0-9]+ ' | awk '{print $3}'`; do sudo umount -f ${dir}; done
rm -fr /tmp/osd*

# clean the etcd config
rm -fr /tmp/etcd-data >/dev/null 2>&1
etcdctl rm --recursive /castle >/dev/null 2>&1
rm -fr /tmp/castle-discovery-url

# ensure castled processes are dead if there was a crash
ps aux | grep castled | grep -E -v 'grep|clean-castled' | awk '{print $2}' | xargs kill >/dev/null 2>&1