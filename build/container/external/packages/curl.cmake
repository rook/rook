
#
# Config
#

# See https://curl.haxx.se/download.html
set(Curl_VERSION 7.51.0)
set(Curl_URL http://curl.askapache.com/download/curl-${Curl_VERSION}.tar.bz2)
set(Curl_URL_SHA256 7f8240048907e5030f67be0a6129bc4b333783b9cca1391026d700835a788dde)

message(STATUS "External: Building Curl ${Curl_VERSION}")

#
# Build
#

set(Curl_CFLAGS "-fPIC -O2")

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

ExternalProject_Add(curl
  PREFIX ${EXTERNAL_ROOT}

  URL ${Curl_URL}
  URL_HASH SHA256=${Curl_URL_SHA256}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND ./configure CFLAGS=${Curl_CFLAGS} --prefix=<INSTALL_DIR> --host=${EXTERNAL_CROSS_TRIPLE} --libdir=<INSTALL_DIR>/lib/${EXTERNAL_CROSS_TRIPLE} ${Curl_Config_Args}
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
