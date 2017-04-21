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

# See https://www.openssl.org/source/
set(Openssl_VERSION 1.1.0c)
set(Openssl_URL https://www.openssl.org/source/openssl-${Openssl_VERSION}.tar.gz)
set(Openssl_URL_SHA256 fc436441a2e05752d31b4e46115eb89709a28aef96d4fe786abe92409b2fd6f5)

message(STATUS "External: Building Cryptopp ${Openssl_VERSION}")

#
# Build
#

string(REGEX REPLACE " " ";" Openssl_CFLAGS "${EXTERNAL_CXX_FLAGS}")

if (${EXTERNAL_CROSS_TRIPLE} STREQUAL "x86_64-linux-gnu")
  set(Openssl_Target linux-x86_64)
elseif(${EXTERNAL_CROSS_TRIPLE} STREQUAL "aarch64-linux-gnu")
  set(Openssl_Target linux-aarch64)
elseif(${EXTERNAL_CROSS_TRIPLE} STREQUAL "arm-linux-gnueabihf")
  set(Openssl_Target linux-armv4)
else()
  message(FATAL_ERROR "openssl: unsupported platform ${EXTERNAL_CROSS_TRIPLE}")
endif()

ExternalProject_Add(openssl
  PREFIX ${EXTERNAL_ROOT}

  URL ${Openssl_URL}
  URL_HASH SHA256=${Openssl_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND <SOURCE_DIR>/Configure --prefix=<INSTALL_DIR> --cross-compile-prefix=${EXTERNAL_CROSS_TRIPLE}- ${Openssl_CFLAGS} ${Openssl_Target}
  BUILD_COMMAND $(MAKE) CC=${CMAKE_C_COMPILER}
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})



