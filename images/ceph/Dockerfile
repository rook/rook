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

# clone the repo. we do this first so that this layer can be reused across archs.
ARG CEPH_GIT_REPO
ARG CEPH_GIT_BRANCH
RUN git clone -b ${CEPH_GIT_BRANCH} --recurse-submodules ${CEPH_GIT_REPO} ceph

# install libraries ceph needs during build
ARG ARCH
ARG CROSS_TRIPLE
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        python-setuptools \
        \
        cython:${ARCH} \
        python:${ARCH} \
        libaio-dev:${ARCH} \
        libatomic-ops-dev:${ARCH} \
        libbabeltrace-dev:${ARCH} \
        libblkid-dev:${ARCH} \
        libboost-context-dev:${ARCH} \
        libboost-coroutine-dev:${ARCH} \
        libboost-date-time-dev:${ARCH} \
        libboost-iostreams-dev:${ARCH} \
        libboost-program-options-dev:${ARCH} \
        libboost-python-dev:${ARCH} \
        libboost-random-dev:${ARCH} \
        libboost-regex-dev:${ARCH} \
        libboost-system-dev:${ARCH} \
        libboost-thread-dev:${ARCH} \
        libcurl4-gnutls-dev:${ARCH} \
        libexpat1-dev:${ARCH} \
        libgoogle-perftools-dev:${ARCH} \
        libgoogle-perftools4:${ARCH} \
        libibverbs-dev:${ARCH} \
        libjemalloc-dev:${ARCH} \
        libkeyutils-dev:${ARCH} \
        libldap2-dev:${ARCH} \
        libnss3-dev:${ARCH} \
        libpython-dev:${ARCH} \
        libsnappy-dev:${ARCH} \
        libssl-dev:${ARCH} \
        libtcmalloc-minimal4:${ARCH} \
        libudev-dev:${ARCH} \
        libunwind-dev:${ARCH} \
        zlib1g-dev:${ARCH} && \
    DEBIAN_FRONTEND=noninteractive apt-get upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# checkout the commit hash and update submodules
ARG CEPH_GIT_COMMIT
RUN cd /build/ceph && \
    git fetch --all --prune && \
    git checkout -b ceph-builder ${CEPH_GIT_COMMIT} && \
    git submodule update --init --recursive

# create a wrapper CMakelists.txt to enable partial building
# and installation of targets. We dont want to build all of
# ceph -- just a subset of targets that we need.
# see http://stackoverflow.com/questions/17164731/installing-only-one-target-and-its-dependencies-out-of-a-complex-project-with
RUN mv /build/ceph/CMakeLists.txt /build/ceph/CMakeLists.original.txt
ADD CMakeLists.txt /build/ceph

# configure ceph
ARG CEPH_BUILD_TYPE
ARG CEPH_ALLOCATOR
RUN mkdir -p /build/ceph/build && cd /build/ceph/build && \
    cmake \
    -DCMAKE_SKIP_INSTALL_ALL_DEPENDENCY=ON \
    -DCMAKE_INSTALL_PREFIX=/usr/local \
    -DCMAKE_BUILD_TYPE=${CEPH_BUILD_TYPE} \
    -DCMAKE_TOOLCHAIN_FILE=/usr/local/toolchain/${CROSS_TRIPLE}.cmake \
    -DALLOCATOR=${CEPH_ALLOCATOR} \
    -DWITH_SYSTEM_BOOST=ON \
    -DWITH_EMBEDDED=OFF \
    -DWITH_FUSE=OFF \
    -DWITH_LEVELDB=OFF \
    -DWITH_LTTNG=OFF \
    -DWITH_MANPAGE=OFF \
    -DWITH_PROFILER=OFF \
    -DWITH_PYTHON3=OFF \
    -DWITH_RADOSGW_FCGI_FRONTEND=OFF \
    ..

WORKDIR /build/ceph/build

# now do the actual building. Note we dont build inside the Dockerfile
# since we are unable to bind a volume and use ccache
ADD build-ceph.sh /build/
CMD ["/build/build-ceph.sh"]
