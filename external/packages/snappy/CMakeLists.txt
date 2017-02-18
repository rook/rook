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
set(Snappy_VERSION 1.1.4)
set(Snappy_URL https://github.com/google/snappy/releases/download/${Snappy_VERSION}/snappy-${Snappy_VERSION}.tar.gz)

message(STATUS "External: Building snappy ${Snappy_VERSION}")

#
# Build
#

set(Snappy_CXXFLAGS ${EXTERNAL_CXX_FLAGS})

set(Snappy_Config_Args
  )

ExternalProject_Add(snappy
  PREFIX ${EXTERNAL_ROOT}

  URL ${Snappy_URL}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure ${EXTERNAL_TOOLCHAIN} CXXFLAGS=${Snappy_CXXFLAGS} --prefix=<INSTALL_DIR> --host=${EXTERNAL_CROSS_TRIPLE} ${Snappy_Config_Args}
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
