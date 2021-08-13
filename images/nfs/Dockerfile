# Copyright 2018 The Rook Authors. All rights reserved.
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

#Portions of this file came from https://github.com/mitcdh/docker-nfs-ganesha/blob/master/Dockerfile, which uses the same license.

FROM NFS_BASEIMAGE
# Build ganesha from source, installing deps and removing them in one line.
# Why?
# 1. Root_Id_Squash, only present in >= 2.4.0.3 which is not yet packaged
# 2. Set NFS_V4_RECOV_ROOT to /export
# 3. Use device major/minor as fsid major/minor to work on OverlayFS

RUN DEBIAN_FRONTEND=noninteractive \
 && apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 10353E8834DC57CA \
 && echo "deb http://ppa.launchpad.net/nfs-ganesha/nfs-ganesha-3.0/ubuntu xenial main" > /etc/apt/sources.list.d/nfs-ganesha.list \
 && echo "deb http://ppa.launchpad.net/nfs-ganesha/libntirpc-3.0/ubuntu xenial main" > /etc/apt/sources.list.d/libntirpc.list \
 && apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 13e01b7b3fe869a9 \
 && echo "deb http://ppa.launchpad.net/gluster/glusterfs-6/ubuntu xenial main" > /etc/apt/sources.list.d/glusterfs.list \
 && apt-get update \
 && apt-get install -y netbase nfs-common dbus nfs-ganesha nfs-ganesha-vfs glusterfs-common xfsprogs \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
 && mkdir -p /run/rpcbind /export /var/run/dbus \
 && touch /run/rpcbind/rpcbind.xdr /run/rpcbind/portmap.xdr \
 && chmod 755 /run/rpcbind/* \
 && chown messagebus:messagebus /var/run/dbus

EXPOSE 2049 38465-38467 662 111/udp 111

COPY rook /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/rook"]
CMD [""]
