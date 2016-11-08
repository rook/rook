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
MAINTAINER Bassam Tabbara <bassam.tabbara@quantum.com>

RUN echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu xenial main universe restricted" > /etc/apt/sources.list &&\
    echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu xenial-updates main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=arm64] http://ports.ubuntu.com/ xenial main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=arm64] http://ports.ubuntu.com/ xenial-updates main universe restricted" >> /etc/apt/sources.list &&\
    dpkg --add-architecture arm64 && \
    apt-get update

#
# install build tools, compilers and cross compilers for all supported platforms
#
RUN DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
    build-essential \
    ca-certificates \
    ccache \
    clang \
    cmake \
    crossbuild-essential-arm64 \
    curl \
    docker.io \
    git \
    jq \
    libxml2-dev \
    llvm-dev \
    mingw-w64 \
    patch \
    rsync \
    runit \
    sudo \
    uuid-dev \
    xz-utils \
    yasm \
    zip

# install the OSX cross compilers. see https://github.com/tpoechtrager/osxcross
RUN OSXCROSS_COMMIT_HASH=86879571d33ff7252a55d777609742561dba9953 && \
    OSXCROSS_SDK=MacOSX10.11.sdk.tar.xz && \
    OSXCROSS_SDK_HASH=d75dddc8d820daf38e90a7c8e9771750a8a4371027a2d658a904cffacbf6b09b && \
    git clone https://github.com/quantum/osxcross && \
    git -C osxcross checkout -b castle-build ${OSXCROSS_COMMIT_HASH} && \
    curl -fsSL https://castle-cross-build.s3.amazonaws.com/osxcross/${OSXCROSS_SDK} -o osxcross/tarballs/${OSXCROSS_SDK} && \
    echo "${OSXCROSS_SDK_HASH}  osxcross/tarballs/${OSXCROSS_SDK}" | sha256sum -c - && \
    UNATTENDED=yes OSX_VERSION_MIN=10.6 osxcross/build.sh && \
    mv osxcross/target /usr/local/osxcross && \
    rm -rf osxcross

ENV PATH /usr/local/osxcross/bin:$PATH

# install acbuild and docker2aci
RUN ACBUILD_VERSION=0.4.0 && \
    curl -fsSLO https://github.com/containers/build/releases/download/v${ACBUILD_VERSION}/acbuild-v${ACBUILD_VERSION}.tar.gz && \
    tar -C /usr/local/bin --strip-components=1 -xzf acbuild-v${ACBUILD_VERSION}.tar.gz && \
    rm -f acbuild-v${ACBUILD_VERSION}.tar.gz && \
    DOCKER2ACI_VERSION=0.12.3 && \
    curl -fsSLO https://github.com/appc/docker2aci/releases/download/v${DOCKER2ACI_VERSION}/docker2aci-v${DOCKER2ACI_VERSION}.tar.gz && \
    tar -C /usr/local/bin --strip-components=1 -xzf docker2aci-v${DOCKER2ACI_VERSION}.tar.gz && \
    rm -f docker2aci-v${DOCKER2ACI_VERSION}.tar.gz

#
# install multi-arch packages needed to build ceph
#
RUN for a in amd64 arm64; do \
    DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-overwrite" install -yq --no-install-recommends \
    libatomic-ops-dev:${a} \
    libblkid-dev:${a} \
    libboost-date-time-dev:${a} \
    libboost-dev:${a} \
    libboost-iostreams-dev:${a} \
    libboost-program-options-dev:${a} \
    libboost-random-dev:${a} \
    libboost-regex-dev:${a} \
    libboost-system-dev:${a} \
    libboost-thread-dev:${a} \
    libcurl4-openssl-dev:${a} \
    libexpat1-dev:${a} \
    libkeyutils-dev:${a} \
    libldap2-dev:${a} \
    libleveldb-dev:${a} \
    libsnappy-dev:${a} \
    libssl-dev:${a} \
    libudev-dev:${a} \
    ; done

# Some of the packages we need for ceph are either not available in xenial or dont support multi-arch.
# Hack'em to install in a multi-arch way. Hopefully these will get fixed upstream soon.
# Bugs opened here https://bugs.launchpad.net/~bassamtabbara
RUN curl -fsSLO http://http.us.debian.org/debian/pool/main/libc/libcrypto++/libcrypto++-dev_5.6.4-2_amd64.deb && \
    curl -fsSLO http://http.us.debian.org/debian/pool/main/libc/libcrypto++/libcrypto++-dev_5.6.4-2_arm64.deb && \
    curl -fsSLO http://http.us.debian.org/debian/pool/main/libc/libcrypto++/libcrypto++6_5.6.4-2_amd64.deb && \
    curl -fsSLO http://http.us.debian.org/debian/pool/main/libc/libcrypto++/libcrypto++6_5.6.4-2_arm64.deb && \
    curl -fsSLO http://archive.ubuntu.com/ubuntu/pool/main/libf/libfcgi/libfcgi-dev_2.4.0-8.4_amd64.deb && \
    curl -fsSLO http://ports.ubuntu.com/pool/main/libf/libfcgi/libfcgi-dev_2.4.0-8.4_arm64.deb && \
    curl -fsSLO http://archive.ubuntu.com/ubuntu/pool/main/libf/libfcgi/libfcgi0ldbl_2.4.0-8.4_amd64.deb && \
    curl -fsSLO http://ports.ubuntu.com/pool/main/libf/libfcgi/libfcgi0ldbl_2.4.0-8.4_arm64.deb && \
    for a in amd64 arm64; do \
        apt-get download libaio-dev:${a} 2> /dev/null && \
        apt-get download libaio1:${a} 2> /dev/null && \
        apt-get download libjemalloc-dev:${a} 2> /dev/null && \
        apt-get download libjemalloc1:${a} 2> /dev/null && \
        apt-get download libgoogle-perftools4:${a} 2> /dev/null && \
        apt-get download libgoogle-perftools-dev:${a} 2> /dev/null && \
        apt-get download libtcmalloc-minimal4:${a} 2> /dev/null && \
        apt-get download libunwind-dev:${a} 2> /dev/null && \
        apt-get download libunwind8-dev:${a} 2> /dev/null && \
        apt-get download libunwind8:${a} 2> /dev/null && \
        apt-get download liblzma5:${a} 2> /dev/null; \
    done

RUN for d in *.deb; do \
        dpkg-deb -R ${d} root && \
        arch=$(cat root/DEBIAN/control | awk '/Architecture/ {printf "%s", $2}') && \
        case ${arch} in amd64) triple=x86_64-linux-gnu ;; arm64) triple=aarch64-linux-gnu ;; esac && \
        if test ! -d root/usr/lib/${triple} -a ! -d root/lib/${triple}; then \
            if test -d root/usr/lib; then \
                echo HACK: moving root/usr/lib for package ${d} && \
                mv root/usr/lib root/usr/lib.rename && \
                mkdir -p root/usr/lib && \
                mv root/usr/lib.rename root/usr/lib/${triple}; \
            fi; \
        fi && \
        if ! grep 'Multi-Arch' root/DEBIAN/control >/dev/null; then \
            echo HACK: setting Multi-Arch: same for package ${d}; \
            sed -i '/^Architecture:.*$/a Multi-Arch: same' root/DEBIAN/control; \
        fi && \
        dpkg-deb -b root ${d} && \
        rm -fr root; \
    done && \
    dpkg -i --force-overwrite *.deb && \
    rm -f *.deb

# cleanup apt-get
RUN apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# install go-lang
RUN GO_VERSION=1.7.1 && \
    GO_HASH=43ad621c9b014cde8db17393dc108378d37bc853aa351a6c74bf6432c1bbd182 && \
    curl -fsSL https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz -o golang.tar.gz && \
    echo "${GO_HASH}  golang.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf golang.tar.gz && \
    rm golang.tar.gz
ENV PATH /usr/local/go/bin:$PATH

# precompile the go standard library for all supported platforms and configurations
RUN \
    CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 CC=o64-clang              go install -a std && \
    CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 CC=o64-clang              go install -tags netgo -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go install -a std && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go install -tags netgo -installsuffix static -a std &&\
    CGO_ENABLED=1 GOOS=linux   GOARCH=arm64 CC=aarch64-linux-gnu-gcc  go install -a std && \
    CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 CC=aarch64-linux-gnu-gcc  go install -tags netgo -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc   go install -a std && \
    CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc   go install -tags netgo -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc   go install -installsuffix race -a std

WORKDIR /build
COPY entrypoint.sh rsyncd.sh /build/

ENTRYPOINT [ "/build/entrypoint.sh" ]
