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

RUN yum --assumeyes install \
        net-tools \
        nmap-ncat && \
    yum clean all && rm -rf /tmp/* /var/tmp/*

ARG ARCH
ARG TINI_VERSION

# Run tini as PID 1 and avoid signal handling issues
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-${ARCH} /tini
RUN chmod +x /tini

ADD rook rookflex /usr/local/bin/

ENTRYPOINT ["/tini", "--", "/usr/local/bin/rook"]
CMD [""]
