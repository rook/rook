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

# See http://www.zlib.net
set(Zlib_VERSION 1.2.11)
set(Zlib_URL http://zlib.net/zlib-${Zlib_VERSION}.tar.gz)
set(Zlib_URL_SHA256 c3e5e9fdd5004dcb542feda5ee4f0ff0744628baf8ed2dd5d66f8ca1197cb1a1)

message(STATUS "External: Building Zlib ${Zlib_VERSION}")

#
# Build
#

set(Zlib_CFLAGS ${EXTERNAL_C_FLAGS})

set(Zlib_Config_Args
  )

ExternalProject_Add(zlib
  PREFIX ${EXTERNAL_ROOT}

  URL ${Zlib_URL}
  URL_HASH SHA256=${Zlib_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND bash -c "CHOST=${EXTERNAL_CROSS_TRIPLE} CFLAGS='${Zlib_CFLAGS}' ./configure --prefix=<INSTALL_DIR> ${Zlib_Config_Args}"
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
