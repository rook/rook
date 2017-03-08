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

# Set the ceph allocator to use (jemalloc, tcmalloc, tcmalloc_minimal, libc)
if (NOT ALLOCATOR)
  set(ALLOCATOR tcmalloc_minimal)
endif()

message(STATUS "External: Building Ceph ${CEPH_BRANCH} (allocator ${ALLOCATOR})")

set(Ceph_CMAKE_ARGS
  -DEXTERNAL_ROOT=${EXTERNAL_ROOT}
  -DEXTERNAL_CCACHE=${EXTERNAL_CCACHE}
  -DCMAKE_BUILD_TYPE=${CMAKE_BUILD_TYPE}
  -DCMAKE_TOOLCHAIN_FILE=${CMAKE_TOOLCHAIN_FILE}
  -DCMAKE_POSITION_INDEPENDENT_CODE=${CMAKE_POSITION_INDEPENDENT_CODE}
  -DALLOCATOR=${ALLOCATOR}
  -DUSE_CRYPTOPP=ON
  -DWITH_EMBEDDED=ON
  -DWITH_FUSE=OFF
  -DWITH_LEVELDB=OFF
  -DWITH_LTTNG=OFF
  -DWITH_MANPAGE=OFF
  -DWITH_MGR=OFF
  -DWITH_NSS=OFF
  -DWITH_OPENLDAP=OFF
  -DWITH_PROFILER=OFF
  -DWITH_PYTHON3=OFF
  -DWITH_RADOSGW_ASIO_FRONTEND=OFF
  -DWITH_RADOSGW_FCGI_FRONTEND=OFF
  -DWITH_SSL=OFF
  -DWITH_SYSTEM_BOOST=ON
  -DWITH_SYSTEM_ROCKSDB=ON
  -DWITH_XFS=OFF
  )

if (NOT CEPH_BRANCH STREQUAL "kraken")
  list(APPEND Ceph_CMAKE_ARGS
    -DWITH_KRBD=OFF
    -DWITH_SYSTEM_ZSTD=ON)
endif()

#
# Build
#

ExternalProject_Add(ceph
  DEPENDS aio boost cryptopp curl expat gperftools jemalloc openssl rocksdb snappy util-linux zlib zstd
  PREFIX ${EXTERNAL_ROOT}

  SOURCE_DIR ${CMAKE_CURRENT_SOURCE_DIR}/../../src/ceph-${CEPH_BRANCH}
  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}

  CMAKE_ARGS ${Ceph_CMAKE_ARGS}
  BUILD_COMMAND $(MAKE) cephd
  BUILD_ALWAYS ${EXTERNAL_ALWAYS_BUILD}
  INSTALL_COMMAND cp <BINARY_DIR>/lib/libcephd.a ${EXTERNAL_ROOT}/lib
          COMMAND cp -aL <SOURCE_DIR>/src/include/cephd ${EXTERNAL_ROOT}/include
          COMMAND cp -aL <SOURCE_DIR>/src/include/rados ${EXTERNAL_ROOT}/include
          COMMAND cp -aL <SOURCE_DIR>/src/include/rbd ${EXTERNAL_ROOT}/include

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE OFF
  LOG_BUILD OFF
  LOG_INSTALL ${EXTERNAL_LOGGING})
