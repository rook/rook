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

# See https://github.com/facebook/zstd
set(Zstd_VERSION 1.1.3)
set(Zstd_URL https://github.com/facebook/zstd/archive/v${Zstd_VERSION}.tar.gz)
set(Zstd_SHA256 106c532ae840a6ee4aee5258f04f3acab7b3e09b9e9584ebe94e4fbfd899af0a)

message(STATUS "External: Building zstd ${Zstd_VERSION}")

#
# Build
#

ExternalProject_Add(zstd
  PREFIX ${EXTERNAL_ROOT}

  URL ${Zstd_URL}
  URL_HASH SHA256=${Zstd_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND true
  BUILD_COMMAND $(MAKE) -C <SOURCE_DIR>/lib ${EXTERNAL_TOOLCHAIN} "CFLAGS=${EXTERNAL_C_FLAGS}"
  INSTALL_COMMAND $(MAKE) -C <SOURCE_DIR>/lib PREFIX=<INSTALL_DIR> install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
