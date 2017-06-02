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

# Optional OS and ARCH to build
ifeq ($(origin GOOS), undefined)
GOOS := $(shell go env GOOS)
endif

ifeq ($(origin GOARCH), undefined)
GOARCH := $(shell go env GOARCH)
endif

GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)

# set cross compile options
ifeq ($(GOOS)_$(GOARCH),linux_amd64)
CROSS_TRIPLE = x86_64-linux-gnu
DEBIAN_ARCH = amd64
endif
ifeq ($(GOOS)_$(GOARCH),linux_arm)
GOARM=7
DEBIAN_ARCH = armhf
CROSS_TRIPLE = arm-linux-gnueabihf
endif
ifeq ($(GOOS)_$(GOARCH),linux_arm64)
DEBIAN_ARCH = arm64
CROSS_TRIPLE = aarch64-linux-gnu
endif
ifeq ($(GOOS)_$(GOARCH),darwin_amd64)
CROSS_TRIPLE=x86_64-apple-darwin15
endif
ifeq ($(GOOS)_$(GOARCH),windows_amd64)
CROSS_TRIPLE=x86_64-w64-mingw32
endif

ifneq ($(GOOS)_$(GOARCH),$(GOHOSTOS)_$(GOHOSTARCH))
CC := $(CROSS_TRIPLE)-gcc
CXX := $(CROSS_TRIPLE)-g++
export CC CXX
endif
