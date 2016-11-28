cmake_minimum_required (VERSION 3.0.2)
project (external)

include(ExternalProject)
include(ProcessorCount)

# set the external root directory
set(EXTERNAL_ROOT ${CMAKE_BINARY_DIR})

# set external download/cache dir
set(EXTERNAL_DOWNLOAD_DIR ${PROJECT_SOURCE_DIR}/download)

# set the cross triple used
set(EXTERNAL_CROSS_TRIPLE ${CROSS_TRIPLE})

# set external download/cache dir
set(EXTERNAL_INSTALL_DIR ${CMAKE_INSTALL_PREFIX})

# whether to log the configure, build and install commands
# or have them go to stdout.
option(EXTERNAL_LOGGING "whether to log all external build output to files" ON)

# set the processor count
ProcessorCount(EXTERNAL_PARALLEL_LEVEL)

message(STATUS "External: Building for ${CROSS_TRIPLE}")
message(STATUS "External: Installing to ${EXTERNAL_INSTALL_DIR}")

#
# Build the external projects in order of dependencies
#

include(packages/zlib.cmake)
include(packages/boost.cmake)
include(packages/curl.cmake)
include(packages/util-linux.cmake)

#
# Copy include files and libraries to the destination
#

install(DIRECTORY ${EXTERNAL_ROOT}/include DESTINATION ${EXTERNAL_INSTALL_DIR} USE_SOURCE_PERMISSIONS)
install(DIRECTORY ${EXTERNAL_ROOT}/lib DESTINATION ${EXTERNAL_INSTALL_DIR} USE_SOURCE_PERMISSIONS)
