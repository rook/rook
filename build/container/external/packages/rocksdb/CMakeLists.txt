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

# See https://github.com/google/snappy
# set(Rocksdb_VERSION 4.11.2)
# set(Rocksdb_URL https://github.com/facebook/rocksdb/archive/v${Rocksdb_VERSION}.tar.gz)

message(STATUS "External: Building rocksdb ${Rocksdb_VERSION}")

#
# Build
#

set(Rocksdb_CXXFLAGS ${EXTERNAL_CXX_FLAGS})

set(Rocksdb_Config_Args)

ExternalProject_Add(rocksdb
  DEPENDS zlib snappy
  PREFIX ${EXTERNAL_ROOT}

  GIT_REPOSITORY https://github.com/ceph/rocksdb.git
  GIT_TAG a0deec960f3a8190831c673e5ba998fe6fb7ea90

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND
    patch -p0 < ${CMAKE_CURRENT_SOURCE_DIR}/00-portable.patch
  CONFIGURE_COMMAND true
  BUILD_COMMAND $(MAKE) PORTABLE=1 DISABLE_JEMALLOC=1 "CXX=${CMAKE_CXX_COMPILER}" "EXTRA_CXXFLAGS=${Rocksdb_CXXFLAGS} -I<INSTALL_DIR>/include" static_lib
  INSTALL_COMMAND $(MAKE) INSTALL_PATH=<INSTALL_DIR> install-static

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
