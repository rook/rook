#
# Config
#

# See http://www.zlib.net
set(Zlib_VERSION 1.2.8)
set(Zlib_URL http://zlib.net/zlib-${Zlib_VERSION}.tar.gz)
set(Zlib_URL_MD5 44d667c142d7cda120332623eab69f40)

message(STATUS "External: Building Zlib ${Zlib_VERSION}")

#
# Build
#

set(Zlib_CFLAGS "-fPIC -O2")

set(Zlib_Config_Args
  --static
  )

ExternalProject_Add(zlib
  PREFIX ${EXTERNAL_ROOT}

  URL ${Zlib_URL}
  URL_HASH MD5=${Zlib_URL_MD5}

  DOWNLOAD_DIR ${EXTERNAL_DOWNLOAD_DIR}
  BUILD_IN_SOURCE 1

  PATCH_COMMAND true
  CONFIGURE_COMMAND bash -c "CHOST=${EXTERNAL_CROSS_TRIPLE} CFLAGS='${Zlib_CFLAGS}' ./configure --prefix=<INSTALL_DIR> --libdir=<INSTALL_DIR>/lib/${EXTERNAL_CROSS_TRIPLE} ${Zlib_Config_Args}"
  BUILD_COMMAND $(MAKE)
  INSTALL_COMMAND $(MAKE) install

  LOG_DOWNLOAD ${EXTERNAL_LOGGING}
  LOG_PATCH ${EXTERNAL_LOGGING}
  LOG_CONFIGURE ${EXTERNAL_LOGGING}
  LOG_BUILD ${EXTERNAL_LOGGING}
  LOG_INSTALL ${EXTERNAL_LOGGING})
