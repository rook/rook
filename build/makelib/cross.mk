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

# ====================================================================================
# Makefile helper functions for cross compiling
#

ifeq ($(GOOS)_$(GOARCH),linux_amd64)
CROSS_TRIPLE=x86_64-linux-gnu
CC=$(CROSS_TRIPLE)-gcc
CXX=$(CROSS_TRIPLE)-g++
OBJCOPY=$(CROSS_TRIPLE)-objcopy
endif

ifeq ($(GOOS)_$(GOARCH),linux_arm64)
CROSS_TRIPLE=aarch64-linux-gnu
CC=$(CROSS_TRIPLE)-gcc
CXX=$(CROSS_TRIPLE)-g++
OBJCOPY=$(CROSS_TRIPLE)-objcopy
endif

ifeq ($(GOOS)_$(GOARCH),darwin_amd64)
CROSS_TRIPLE=x86_64-apple-darwin15
CC=$(CROSS_TRIPLE)-clang
CXX=$(CROSS_TRIPLE)-clang++
OBJCOPY=$(CROSS_TRIPLE)-objcopy
endif

ifeq ($(CEPHD_PLATFORM),windows_amd64)
CROSS_TRIPLE=x86_64-w64-mingw32
CC=$(CROSS_TRIPLE)-gcc
CXX=$(CROSS_TRIPLE)-g++
OBJCOPY=$(CROSS_TRIPLE)-objcopy
endif

export CC CXX OBJCOPY
