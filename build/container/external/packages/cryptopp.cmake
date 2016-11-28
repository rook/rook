#
# Config
#

# See https://www.cryptopp.com/downloads.html
set(Cryptopp_VERSION 5.6.5)
set(Cryptopp_URL https://www.cryptopp.com/cryptopp565.zip)
set(Cryptopp_URL_SHA256 a75ef486fe3128008bbb201efee3dcdcffbe791120952910883b26337ec32c34)

message(STATUS "External: Building Cryptopp ${Cryptopp_VERSION}")

#
# Build
#

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
  --disable-shared
  --disable-smb
  --disable-smtp
  --disable-telnet
  --disable-tftp
  --disable-unix-sockets
  --with-ssl
  --without-gssapi
  --without-libssh2
  --without-nss
  --without-winidn
  )

set(Curl_MAKE_ARGS -j${EXTERNAL_PARALLEL_LEVEL})

if(EXTERNAL_VERBOSE)
  list(APPEND Curl_MAKE_ARGS V=1)
endif()

ExternalProject_Add(curl
  PREFIX ${EXTERNAL_ROOT}

  URL ${Curl_URL}
  URL_HASH SHA256=${Curl_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure --prefix=<INSTALL_DIR> ${Curl_Config_Args} --host=${EXTERNAL_CROSS_TRIPLE} --libdir=<INSTALL_DIR>/lib/${EXTERNAL_CROSS_TRIPLE}
  BUILD_COMMAND make ${Curl_MAKE_ARGS}
  INSTALL_COMMAND make ${Curl_MAKE_ARGS} install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
