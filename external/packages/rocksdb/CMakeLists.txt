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

set(Rocksdb_HASH a0deec960f3a8190831c673e5ba998fe6fb7ea90)
set(Rocksdb_URL https://github.com/facebook/rocksdb/archive/${Rocksdb_HASH}.zip)


message(STATUS "External: Building rocksdb ${Rocksdb_HASH}")

#
# Build
#

set(Rocksdb_CFLAGS "${EXTERNAL_C_FLAGS} -I<INSTALL_DIR>/include -L<INSTALL_DIR>/lib")
set(Rocksdb_CXXFLAGS "${EXTERNAL_CXX_FLAGS} -I<INSTALL_DIR>/include -L<INSTALL_DIR>/lib")
set(Rocksdb_LDFLAGS "${EXTERNAL_LD_FLAGS} -L<INSTALL_DIR>/lib")

ExternalProject_Add(rocksdb
  DEPENDS zlib snappy zstd
  PREFIX ${EXTERNAL_ROOT}

  URL ${Rocksdb_URL}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  DOWNLOAD_NAME rocksdb-${Rocksdb_HASH}.zip
  BUILD_IN_SOURCE 1
  PATCH_COMMAND patch --batch -p0 < ${CMAKE_CURRENT_SOURCE_DIR}/00-portable.patch
        COMMAND patch --batch -p0 < ${CMAKE_CURRENT_SOURCE_DIR}/01-cxx-detect.patch
  CONFIGURE_COMMAND true
  BUILD_COMMAND $(MAKE) DEBUG_LEVEL=1 PORTABLE=1 DISABLE_JEMALLOC=1 LIBNAME=librocksdb "CXX=${CMAKE_CXX_COMPILER}" "EXTRA_CFLAGS=${Rocksdb_CFLAGS}" "EXTRA_CXXFLAGS=${Rocksdb_CXXFLAGS}" "EXTRA_LDFLAGS=${Rocksdb_LDFLAGS}" static_lib
  INSTALL_COMMAND $(MAKE) LIBNAME=librocksdb INSTALL_PATH=<INSTALL_DIR> install-static

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
