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

#Portions of this file came from https://github.com/kubernetes-incubator/external-storage/blob/master/nfs/deploy/docker/Dockerfile, which uses the same license.

FROM fedora:25

# Build ganesha from source, installing deps and removing them in one line.
# Why?
# 1. Root_Id_Squash, only present in >= 2.4.0.3 which is not yet packaged
# 2. Set NFS_V4_RECOV_ROOT to /export
# 3. Use device major/minor as fsid major/minor to work on OverlayFS

RUN dnf install -y tar gcc cmake autoconf libtool bison flex make gcc-c++ krb5-devel dbus-devel jemalloc-devel libnfsidmap-devel patch && dnf clean all \
 && curl -L https://github.com/nfs-ganesha/nfs-ganesha/archive/V2.4.1.tar.gz | tar zx \
 && curl -L https://github.com/nfs-ganesha/nfs-ganesha/commit/c583534b166be198755d905c82a7687d83b458d8.patch -o /nfs-ganesha.patch \
 && curl -L https://github.com/nfs-ganesha/ntirpc/archive/v1.4.4.tar.gz | tar zx \
 && rm -r nfs-ganesha-2.4.1/src/libntirpc \
 && mv ntirpc-1.4.4 nfs-ganesha-2.4.1/src/libntirpc \
 && cd nfs-ganesha-2.4.1 \
 && cmake -DCMAKE_BUILD_TYPE=Release -DBUILD_CONFIG=vfs_only src/ \
 && patch -p1 < /nfs-ganesha.patch \
 && make \
 && make install \
 && cp src/scripts/ganeshactl/org.ganesha.nfsd.conf /etc/dbus-1/system.d/ \
 && cd .. \
 && rm -rf nfs-ganesha-2.4.1 /nfs-ganesha.patch \
 && dnf remove -y tar gcc cmake autoconf libtool bison flex make gcc-c++ krb5-devel dbus-devel jemalloc-devel libnfsidmap-devel patch && dnf clean all

RUN dnf install -y dbus-x11 rpcbind hostname nfs-utils xfsprogs jemalloc libnfsidmap && dnf clean all

RUN mkdir -p /var/run/dbus
RUN mkdir -p /export

COPY start.sh /

EXPOSE 2049 38465-38467 662 111/udp 111

ADD rook /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/rook"]
CMD [""]
