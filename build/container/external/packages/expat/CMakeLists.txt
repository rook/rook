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

# See http://expat.sourceforge.net
set(Expat_VERSION 2.2.0)
set(Expat_URL https://sourceforge.net/projects/expat/files/expat/${Expat_VERSION}/expat-${Expat_VERSION}.tar.bz2)
set(Expat_URL_SHA1 8453bc52324be4c796fd38742ec48470eef358b3)

message(STATUS "External: Building util-linux ${Expat_VERSION}")

#
# Build
#

set(Expat_CFLAGS ${EXTERNAL_C_FLAGS})

set(Expat_Config_Args
  )

ExternalProject_Add(expat
  PREFIX ${EXTERNAL_ROOT}

  URL ${Expat_URL}
  URL_HASH SHA1=${Expat_URL_SHA1}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure CFLAGS=${Expat_CFLAGS} --prefix=<INSTALL_DIR> --host=${EXTERNAL_CROSS_TRIPLE} ${Expat_Config_Args}
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
