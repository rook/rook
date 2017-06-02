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

ARG EXTRALIBS

# install runtime dependencies for ceph and rookd
RUN BOOST_VERSION=1.62.0 && \
    DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        ca-certificates \
        libaio1 \
        libboost-context${BOOST_VERSION} \
        libboost-coroutine${BOOST_VERSION} \
        libboost-date-time${BOOST_VERSION} \
        libboost-iostreams${BOOST_VERSION} \
        libboost-program-options${BOOST_VERSION} \
        libboost-python${BOOST_VERSION} \
        libboost-random${BOOST_VERSION} \
        libboost-regex${BOOST_VERSION} \
        libboost-system${BOOST_VERSION} \
        libboost-thread${BOOST_VERSION} \
        libcurl3-gnutls \
        libexpat1 \
        libibverbs1 \
        libldap-2.4-2 \
        libnss3 \
        libpython2.7 \
        python-prettytable \
        python2.7-minimal \
        ${EXTRALIBS} && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# add the ceph binaries
ADD ceph-install.tar /

# ensure dynamic libraries are updated
RUN ldconfig

ENV PYTHONPATH=$PYTHONPATH:/usr/local/lib/python2.7/site-packages

ADD rookd /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/rookd"]
