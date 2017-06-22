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

# install apt-cacher-ng
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        apt-cacher-ng && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN ln -sf /dev/stdout /var/log/apt-cacher-ng/apt-cacher.out && \
    ln -sf /dev/stderr /var/log/apt-cacher-ng/apt-cacher.err && \
    echo 'http://archive.ubuntu.com/ubuntu/' > /etc/apt-cacher-ng/backends_ubuntu && \
    echo 'http://ports.ubuntu.com/' > /etc/apt-cacher-ng/backends_ubuntu

VOLUME ["/var/cache/apt-cacher-ng"]
EXPOSE 3142

CMD [ "/usr/sbin/apt-cacher-ng", "ForeGround=1", "Port=3142", "CacheDir=/var/cache/apt-cacher-ng"]
