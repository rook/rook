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

# see Makefile for the BASEIMAGE definition
FROM BASEIMAGE

ARG ARCH
ARG TINI_VERSION

# Run tini as PID 1 and avoid signal handling issues
RUN curl --fail -sSL -o /tini https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${ARCH} && \
    chmod +x /tini && \
    yum install make gcc perl pcre-devel zlib-devel -y && \
    yum install curl -y && \
    curl -sSL -o openssl-1.1.1.tar.gz https://ftp.openssl.org/source/old/1.1.1/openssl-1.1.1.tar.gz && \
    tar xvf openssl-1.1.1.tar.gz && \
    cd openssl-1.1.1/ && \
    ./config --prefix=/usr --openssldir=/etc/ssl --libdir=lib no-shared zlib-dynamic && \
    make && \
    make test && \
    make install && \
    export LD_LIBRARY_PATH=/usr/local/lib:/usr/local/lib64 && \
    echo "export LD_LIBRARY_PATH=/usr/local/lib:/usr/local/lib64" >> ~/.bashrc && \
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin


COPY rook rookflex toolbox.sh /usr/local/bin/
COPY ceph-csi /etc/ceph-csi
COPY ceph-monitoring /etc/ceph-monitoring
COPY rook-external /etc/rook-external/
COPY ceph-csv-templates /etc/ceph-csv-templates
COPY adm-con /etc/rook/adm-con/
ENTRYPOINT ["/tini", "--", "/usr/local/bin/rook"]
CMD [""]
