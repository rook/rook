#!/bin/bash


sudo rm -fr /var/lib/castle
sudo rm -fr /var/log/castle

# clean the etcd config
rm -fr /tmp/etcd-data >/dev/null 2>&1
etcdctl rm --recursive /castle >/dev/null 2>&1
rm -fr /tmp/castle-discovery-url

# clean the castle data dir
rm -fr /tmp/castle >/dev/null 2>&1

# ensure castled processes are dead if there was a crash
ps aux | grep castled | grep -E -v 'grep|clean-castled' | awk '{print $2}' | xargs kill >/dev/null 2>&1

# clear the data partitions
for DEV in sdb sdc sdd; do
    sudo sgdisk --zap-all /dev/$DEV
done

