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

FROM BASEIMAGE

# install build tools, compilers and cross compilers for all supported platforms
RUN echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu zesty main universe restricted" > /etc/apt/sources.list &&\
    echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu zesty-updates main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu zesty-security main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=amd64] http://archive.ubuntu.com/ubuntu zesty-backports main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=armhf,arm64] http://ports.ubuntu.com/ zesty main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=armhf,arm64] http://ports.ubuntu.com/ zesty-updates main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=armhf,arm64] http://ports.ubuntu.com/ zesty-security main universe restricted" >> /etc/apt/sources.list &&\
    echo "deb [arch=armhf,arm64] http://ports.ubuntu.com/ zesty-backports main universe restricted" >> /etc/apt/sources.list &&\
    dpkg --add-architecture armhf && \
    dpkg --add-architecture arm64 && \
    DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        build-essential \
        ca-certificates \
        ccache \
        cmake \
        crossbuild-essential-arm64 \
        crossbuild-essential-armhf \
        curl \
        git \
        jq \
        patch \
        yasm \
        zip && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# add cmake toolchains
ADD toolchain /usr/local/toolchain

# set the working directory
WORKDIR /build

ENV TERM=xterm
