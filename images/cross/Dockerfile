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
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        awscli \
        curl \
        docker.io \
        lsb-release \
        rsync \
        runit \
        sudo \
        xz-utils && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# install acbuild and docker2aci
RUN ACBUILD_VERSION=0.4.0 && \
    curl -fsSLO https://github.com/containers/build/releases/download/v${ACBUILD_VERSION}/acbuild-v${ACBUILD_VERSION}.tar.gz && \
    tar -C /usr/local/bin --strip-components=1 -xzf acbuild-v${ACBUILD_VERSION}.tar.gz && \
    rm -f acbuild-v${ACBUILD_VERSION}.tar.gz && \
    DOCKER2ACI_VERSION=0.14.0 && \
    curl -fsSLO https://github.com/appc/docker2aci/releases/download/v${DOCKER2ACI_VERSION}/docker2aci-v${DOCKER2ACI_VERSION}.tar.gz && \
    tar -C /usr/local/bin --strip-components=1 -xzf docker2aci-v${DOCKER2ACI_VERSION}.tar.gz && \
    rm -f docker2aci-v${DOCKER2ACI_VERSION}.tar.gz

COPY run.sh rsyncd.sh /build/
CMD [ "/build/run.sh" ]
