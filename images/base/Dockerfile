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

# APT caching to speed up apt-get downloads
# see http://www.stuartaxon.com/2015/02/02/docker-and-caching-apt-get-for-guests-on-the-same-host/
RUN HOST_IP=$(route -n | awk '/^0.0.0.0/ {print $2}') && \
    echo "#!/bin/bash" > /usr/local/bin/apt-ng-host-discover && \
    echo "if nc -w1 -z $HOST_IP 3142; then printf http://$HOST_IP:3142; else printf DIRECT; fi" >> /usr/local/bin/apt-ng-host-discover && \
    chmod +x /usr/local/bin/apt-ng-host-discover && \
    echo 'Acquire::http::ProxyAutoDetect "/usr/local/bin/apt-ng-host-discover";' > /etc/apt/apt.conf.d/30proxy

CMD ["/bin/bash"]
