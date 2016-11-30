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

# See https://www.cryptopp.com/downloads.html
set(Cryptopp_VERSION 5.6.5)
set(Cryptopp_URL https://www.cryptopp.com/cryptopp565.zip)
set(Cryptopp_URL_SHA256 a75ef486fe3128008bbb201efee3dcdcffbe791120952910883b26337ec32c34)

message(STATUS "External: Building Cryptopp ${Cryptopp_VERSION}")

#
# Build
#

set(Cryptopp_CXXFLAGS ${EXTERNAL_CXX_FLAGS})
set(Cryptopp_TOOLCHAIN CXX=${CMAKE_CXX_COMPILER} AR=${CMAKE_AR} RANLIB=${CMAKE_RANLIB})

set(Cryptopp_MAKE_ARGS)

ExternalProject_Add(cryptopp
  PREFIX ${EXTERNAL_ROOT}

  URL ${Cryptopp_URL}
  URL_HASH SHA256=${Cryptopp_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND true
  BUILD_COMMAND $(MAKE) CXXFLAGS=${Cryptopp_CXXFLAGS} ${Cryptopp_TOOLCHAIN} static
  INSTALL_COMMAND $(MAKE) PREFIX=<INSTALL_DIR> install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
