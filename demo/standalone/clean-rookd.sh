#!/bin/bash

# Copyright 2016 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

sudo rm -fr /var/lib/rook
sudo mkdir /var/lib/rook;sudo chown -R $USER:$USER /var/lib/rook
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
    sudo sgdisk --zap-all /dev/$DEV >/dev/null 2>&1
done

