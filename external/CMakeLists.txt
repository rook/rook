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

cmake_minimum_required (VERSION 3.4)
project (external)

include(ExternalProject)
include(ProcessorCount)

# set the external root directory
set(EXTERNAL_ROOT ${CMAKE_BINARY_DIR})

# set external download/cache dir
set(EXTERNAL_DOWNLOAD_DIR ${EXTERNAL_ROOT}/download CACHE STRING "where to cache the download packages")

# set the cross triple used
set(EXTERNAL_CROSS_TRIPLE ${CROSS_TRIPLE})

# set always build option
set(EXTERNAL_ALWAYS_BUILD 1 CACHE STRING "whether to force build some packages like ceph")

# whether to log the configure, and build commands
# or have them go to stdout.
option(EXTERNAL_LOGGING "whether to log all external build output to files" ON)

# the version of ceph to build
set(CEPH_BRANCH "kraken" CACHE STRING "the branch of ceph to build")

# set the processor count
# FIX: this should really be done at runtime with make -j4. Boost requires this
# now since it goes inside ExternalProject_Add
ProcessorCount(EXTERNAL_PARALLEL_LEVEL)

# set build type and compile lags
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE DEBUG)
endif()

string(TOUPPER "${CMAKE_BUILD_TYPE}" build_type_upper)
set(EXTERNAL_C_FLAGS "${CMAKE_C_FLAGS} ${CMAKE_C_FLAGS_${build_type_upper}}")
set(EXTERNAL_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_CXX_FLAGS_${build_type_upper}}")
set(EXTERNAL_LD_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_LD_FLAGS_${build_type_upper}}")

set(EXTERNAL_TOOLCHAIN CC=${CMAKE_C_COMPILER} CXX=${CMAKE_CXX_COMPILER} AR=${CMAKE_AR} RANLIB=${CMAKE_RANLIB})

message(STATUS "External: Building for ${CROSS_TRIPLE}")
message(STATUS "External: Build Type: ${CMAKE_BUILD_TYPE}")
message(STATUS "External: CFLAGS: ${EXTERNAL_C_FLAGS}")
message(STATUS "External: CXXFLAGS: ${EXTERNAL_CXX_FLAGS}")
message(STATUS "External: LDFLAGS: ${EXTERNAL_LD_FLAGS}")

#
# Build the external projects
#

add_subdirectory(packages/zlib)
add_subdirectory(packages/zstd)
add_subdirectory(packages/snappy)
add_subdirectory(packages/openssl)
add_subdirectory(packages/aio)
add_subdirectory(packages/boost)
add_subdirectory(packages/cryptopp)
add_subdirectory(packages/curl)
add_subdirectory(packages/expat)
add_subdirectory(packages/gperftools)
add_subdirectory(packages/jemalloc)
add_subdirectory(packages/util-linux)
add_subdirectory(packages/rocksdb)
add_subdirectory(packages/ceph)
