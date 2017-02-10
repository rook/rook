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

set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_VERSION 1)

set(CMAKE_C_COMPILER "/usr/bin/${CROSS_TRIPLE}-gcc" CACHE PATH "C compiler")
set(CMAKE_CXX_COMPILER "/usr/bin/${CROSS_TRIPLE}-g++" CACHE PATH "C++ compiler")
set(CMAKE_ASM_COMPILER "/usr/bin/${CROSS_TRIPLE}-gcc" CACHE PATH "assembler")
set(CMAKE_STRIP "/usr/bin/${CROSS_TRIPLE}-strip" CACHE PATH "strip")
set(CMAKE_AR "/usr/bin/${CROSS_TRIPLE}-ar" CACHE PATH "archive")
set(CMAKE_LINKER "/usr/bin/${CROSS_TRIPLE}-ld" CACHE PATH "linker")
set(CMAKE_NM "/usr/bin/${CROSS_TRIPLE}-nm" CACHE PATH "nm")
set(CMAKE_OBJCOPY "/usr/bin/${CROSS_TRIPLE}-objcopy" CACHE PATH "objcopy")
set(CMAKE_OBJDUMP "/usr/bin/${CROSS_TRIPLE}-objdump" CACHE PATH "objdump")
set(CMAKE_RANLIB "/usr/bin/${CROSS_TRIPLE}-ranlib" CACHE PATH "ranlib")

set(CMAKE_FIND_ROOT_PATH /usr/${CROSS_TRIPLE})

set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)

# the toolchain file is included multiple times during config.
# this ensures that the flags are set only once
if(DEFINED TOOLCHAIN_FLAGS_CONFIGURED)
  return()
else()
  set(TOOLCHAIN_FLAGS_CONFIGURED 1)
endif()

set(CMAKE_C_FLAGS_DEBUG "-g" CACHE STRING "c Debug flags" FORCE)
set(CMAKE_CXX_FLAGS_DEBUG "-g" CACHE STRING "c++ Debug flags" FORCE)
set(CMAKE_C_FLAGS_RELEASE "-O3 -DNDEBUG" CACHE STRING "c Release flags" FORCE)
set(CMAKE_CXX_FLAGS_RELEASE "-O3 -DNDEBUG" CACHE STRING "c++ Release flags" FORCE)
set(CMAKE_C_FLAGS_MINSIZEREL "-Os -DNDEBUG" CACHE STRING "c Release min size flags" FORCE)
set(CMAKE_CXX_FLAGS_MINSIZEREL "-Os -DNDEBUG" CACHE STRING "c++ Release min size flags" FORCE)
set(CMAKE_C_FLAGS_RELWITHDEBINFO "-g -O2 -DNDEBUG" CACHE STRING "c Release with debug info flags" FORCE)
set(CMAKE_CXX_FLAGS_RELWITHDEBINFO "-g -O2 -DNDEBUG" CACHE STRING "c++ Release with debug info flags" FORCE)

if (CMAKE_POSITION_INDEPENDENT_CODE)
  set(CMAKE_C_FLAGS_DEBUG "${CMAKE_C_FLAGS_DEBUG} -fPIC" CACHE STRING "c Debug flags" FORCE)
  set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -fPIC" CACHE STRING "c++ Debug flags" FORCE)
  set(CMAKE_C_FLAGS_RELEASE "${CMAKE_C_FLAGS_RELEASE} -fPIC" CACHE STRING "c Release flags" FORCE)
  set(CMAKE_CXX_FLAGS_RELEASE "${CMAKE_CXX_FLAGS_RELEASE} -fPIC" CACHE STRING "c++ Release flags" FORCE)
  set(CMAKE_C_FLAGS_MINSIZEREL "${CMAKE_C_FLAGS_MINSIZEREL} -fPIC" CACHE STRING "c Release min size flags" FORCE)
  set(CMAKE_CXX_FLAGS_MINSIZEREL "${CMAKE_CXX_FLAGS_MINSIZEREL} -fPIC" CACHE STRING "c Release min size flags" FORCE)
  set(CMAKE_C_FLAGS_RELWITHDEBINFO "${CMAKE_C_FLAGS_RELWITHDEBINFO} -fPIC" CACHE STRING "c Release with debug info flags" FORCE)
  set(CMAKE_CXX_FLAGS_RELWITHDEBINFO "${CMAKE_CXX_FLAGS_RELWITHDEBINFO} -fPIC" CACHE STRING "c++ Release with debug info flags" FORCE)
endif()
