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
