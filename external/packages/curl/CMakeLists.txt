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

# See https://curl.haxx.se/download.html
set(Curl_VERSION 7.51.0)
set(Curl_URL http://curl.askapache.com/download/curl-${Curl_VERSION}.tar.bz2)
set(Curl_URL_SHA256 7f8240048907e5030f67be0a6129bc4b333783b9cca1391026d700835a788dde)

message(STATUS "External: Building Curl ${Curl_VERSION}")

#
# Build
#

set(Curl_CFLAGS "${EXTERNAL_C_FLAGS} -I<INSTALL_DIR>/include")
set(Curl_LDFLAGS "${EXTERNAL_LD_FLAGS} -L<INSTALL_DIR>/lib")

set(Curl_Config_Args
  --disable-cookies
  --disable-crypto-auth
  --disable-dict
  --disable-ftp
  --disable-gopher
  --disable-imap
  --disable-ldap
  --disable-manual
  --disable-pop3
  --disable-rtsp
  --disable-smb
  --disable-smtp
  --disable-telnet
  --disable-tftp
  --disable-unix-sockets
  --without-gssapi
  --without-libssh2
  --without-nss
  --without-winidn
  )

ExternalProject_Add(curl
  DEPENDS zlib openssl
  PREFIX ${EXTERNAL_ROOT}

  URL ${Curl_URL}
  URL_HASH SHA256=${Curl_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure ${EXTERNAL_TOOLCHAIN} "CFLAGS=${Curl_CFLAGS}" "LDFLAGS=${Curl_LDFLAGS}" --prefix=<INSTALL_DIR> --with-ssl=<INSTALL_DIR>/lib --host=${EXTERNAL_CROSS_TRIPLE} ${Curl_Config_Args}
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
