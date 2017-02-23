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

FROM ubuntu:xenial
MAINTAINER Jared Watts <jared.watts@quantum.com>

# install common tools for debugging and testing (including ceph Kraken)
RUN apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 08b73419ac32b4e966c1a330e84ac2c0460f3994 \
    && echo "deb http://download.ceph.com/debian-kraken/ xenial main" > /etc/apt/sources.list.d/ceph-kraken.list \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
        binutils \
        ceph-common \
        curl \
        fio \
        gdb \
        iperf3 \
        jq \
        less \
        man-db \
        vim \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

ENV ROOK_VERSION v0.2.2
RUN cd /tmp && \
    curl -s -L https://github.com/rook/rook/releases/download/${ROOK_VERSION}/rook-${ROOK_VERSION}-linux-amd64.tar.gz > rook.tar.gz && \
    tar xvf rook.tar.gz && \
    mv rook /usr/bin/ && \
    chown root:root /usr/bin/rook && \
    rm -fr rook*

COPY entrypoint.sh /toolbox/
ENTRYPOINT [ "/toolbox/entrypoint.sh" ]
