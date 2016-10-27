#!/bin/bash


sudo rm -fr /var/lib/rook
sudo rm -fr /var/log/rook

# clean the etcd config
rm -fr /tmp/etcd-data >/dev/null 2>&1
etcdctl rm --recursive /rook >/dev/null 2>&1
rm -fr /tmp/rook-discovery-url

# clean the rook data dir
rm -fr /tmp/rook >/dev/null 2>&1

# ensure rookd processes are dead if there was a crash
ps aux | grep rookd | grep -E -v 'grep|clean-rookd' | awk '{print $2}' | xargs kill >/dev/null 2>&1

# clear the data partitions
for DEV in sdb sdc sdd; do
    sudo sgdisk --zap-all /dev/$DEV
done

