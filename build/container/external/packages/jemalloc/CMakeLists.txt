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
set(Jemalloc_VERSION 4.3.1)
set(Jemalloc_URL https://github.com/jemalloc/jemalloc/releases/download/${Jemalloc_VERSION}/jemalloc-${Jemalloc_VERSION}.tar.bz2)

message(STATUS "External: Building jemalloc ${Jemalloc_VERSION}")

#
# Build
#

#FIX: strip jemalloc since we are unable to remove the g3.

set(Jemalloc_CFLAGS ${EXTERNAL_C_FLAGS})

set(Jemalloc_Config_Args)

ExternalProject_Add(jemalloc
  PREFIX ${EXTERNAL_ROOT}

  URL ${Jemalloc_URL}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure EXTRA_CFLAGS=${Jemalloc_CFLAGS} --prefix=<INSTALL_DIR> --host=${EXTERNAL_CROSS_TRIPLE} ${Jemalloc_Config_Args}
  BUILD_COMMAND $(MAKE) build_lib_static build_lib_shared
  INSTALL_COMMAND $(MAKE) install_include install_lib_static install_lib_shared

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
