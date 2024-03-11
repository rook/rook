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

# env vars for s5cmd
ARG S5CMD_VERSION
ARG S5CMD_ARCH

# install 'ip' tool for Multus
RUN dnf install -y --repo baseos --setopt=install_weak_deps=False iproute && dnf clean all

# Install the s5cmd package to interact with s3 gateway
RUN curl --fail -sSL -o /s5cmd.tar.gz https://github.com/peak/s5cmd/releases/download/v${S5CMD_VERSION}/s5cmd_${S5CMD_VERSION}_${S5CMD_ARCH}.tar.gz && \
    mkdir /s5cmd && \
    tar xf /s5cmd.tar.gz -C /s5cmd && \
    install /s5cmd/s5cmd /usr/local/bin/s5cmd && \
    rm -rf /s5cmd.tar.gz /s5cmd

COPY rook toolbox.sh set-ceph-debug-level /usr/local/bin/
COPY ceph-monitoring /etc/ceph-monitoring
COPY rook-external /etc/rook-external/
RUN useradd rook -u 2016 # 2016 is the UID of the rook user and also the year of the first commit in the project
USER 2016
ENTRYPOINT ["/usr/local/bin/rook"]
CMD [""]
