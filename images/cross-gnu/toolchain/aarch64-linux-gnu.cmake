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
set(CMAKE_SYSTEM_PROCESSOR aarch64)

set(cross_triple "aarch64-linux-gnu")

set(CMAKE_C_COMPILER "/usr/lib/ccache/${cross_triple}-gcc" CACHE PATH "C compiler")
set(CMAKE_CXX_COMPILER "/usr/lib/ccache/${cross_triple}-g++" CACHE PATH "C++ compiler")
set(CMAKE_ASM_COMPILER "/usr/bin/${cross_triple}-gcc" CACHE PATH "assembler")
set(CMAKE_STRIP "/usr/bin/${cross_triple}-strip" CACHE PATH "strip")
set(CMAKE_AR "/usr/bin/${cross_triple}-ar" CACHE PATH "archive")
set(CMAKE_LINKER "/usr/bin/${cross_triple}-ld" CACHE PATH "linker")
set(CMAKE_NM "/usr/bin/${cross_triple}-nm" CACHE PATH "nm")
set(CMAKE_OBJCOPY "/usr/bin/${cross_triple}-objcopy" CACHE PATH "objcopy")
set(CMAKE_OBJDUMP "/usr/bin/${cross_triple}-objdump" CACHE PATH "objdump")
set(CMAKE_RANLIB "/usr/bin/${cross_triple}-ranlib" CACHE PATH "ranlib")

set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)

set(CMAKE_CROSSCOMPILING_EMULATOR /usr/bin/qemu-aarch64-static)
