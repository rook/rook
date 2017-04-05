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

# See https://github.com/gperftools/gperftools
set(Gperftools_VERSION 2.5)
set(Gperftools_URL https://github.com/gperftools/gperftools/releases/download/gperftools-${Gperftools_VERSION}/gperftools-${Gperftools_VERSION}.tar.gz)
set(Gperftools_SHA256 6fa2748f1acdf44d750253e160cf6e2e72571329b42e563b455bde09e9e85173)

message(STATUS "External: Building gperftools ${Gperftools_VERSION}")

#
# Build
#

set(Gperftools_CXXFLAGS ${EXTERNAL_CXX_FLAGS})

set(Gperftools_Config_Args
  )

ExternalProject_Add(gperftools
  PREFIX ${EXTERNAL_ROOT}

  URL ${Gperftools_URL}
  URL_HASH SHA256=${Gperftools_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure ${EXTERNAL_TOOLCHAIN} CXXFLAGS=${Gperftools_CXXFLAGS} --prefix=<INSTALL_DIR> --host=${EXTERNAL_CROSS_TRIPLE} ${Gperftools_Config_Args}
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
