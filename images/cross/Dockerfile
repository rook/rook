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

FROM ubuntu:xenial-20210114

# install build tools, compilers and cross compilers for all supported platforms
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
    awscli \
    ca-certificates \
    curl \
    docker.io \
    git \
    jq \
    lsb-release \
    make \
    rsync \
    runit \
    sudo \
    zip && \
    curl -sL https://deb.nodesource.com/setup_10.x | bash - && \
    DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends nodejs && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# install golang from the official repo
RUN GO_VERSION=1.16.7 && \
    GO_HASH=7fe7a73f55ba3e2285da36f8b085e5c0159e9564ef5f63ee0ed6b818ade8ef04 && \
    curl -fsSL https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz -o golang.tar.gz && \
    echo "${GO_HASH}  golang.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf golang.tar.gz && \
    rm golang.tar.gz
ENV PATH /usr/local/go/bin:$PATH

ARG ARCH
ARG TINI_VERSION

# Run tini as PID 1 and avoid signal handling issues
RUN curl -sSL --fail -o /tini https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${ARCH} && \
    chmod +x /tini

# precompile the go standard library for all supported platforms and configurations
# the install suffixes match those in golang.mk so please keep them in sync
RUN platforms="linux_amd64 linux_arm64" && \
    for p in $platforms; do CGO_ENABLED=0 GOOS=${p%_*} GOARCH=${p##*_} GOARM=7 go install -installsuffix static -a std; done

COPY run.sh rsyncd.sh /build/

ENTRYPOINT [ "/tini", "-g", "--", "/build/run.sh" ]
