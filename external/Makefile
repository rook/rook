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

.PHONY: all build
all: build

# ====================================================================================
# Options

# Set the memory allocator used for ceph
ALLOCATOR ?= tcmalloc_minimal

# The release of ceph to build
CEPH_BRANCH ?= kraken

# Whether to build ceph everytime and ignore the timestamps
ALWAYS_BUILD ?= ON

# The platforms supported for cross building
PLATFORMS ?= x86_64-linux-gnu aarch64-linux-gnu

# The build type. Debug produces non-optimized builds. Release
# produces fully optimized builds. RelWithDebInfo produces
# optimized builds with debug information.
BUILD_TYPE ?= RelWithDebInfo

# Verbose build
V ?= 0
ifeq ($(V),1)
	EXTERNAL_LOGGING=OFF
	MAKEFLAGS += VERBOSE=1 V=1
else
	EXTERNAL_LOGGING=ON
	MAKEFLAGS += --no-print-directory
endif

# Whether to use ccache
CCACHE ?= 1
ifeq ($(CCACHE),1)
	EXTERNAL_CCACHE=ON
endif

# The build and download directories
ROOTDIR ?= $(CURDIR)
BUILDDIR ?= $(ROOTDIR)/build
DOWNLOADDIR ?= $(ROOTDIR)/download

# The host platform
HOST = $(shell gcc -dumpmachine)

# ====================================================================================
# Targets

define build-platform
build.$(1):
	@mkdir -p $(DOWNLOADDIR)
	@cmake \
		-H$(CURDIR) \
		-B$(BUILDDIR)/$(1) \
		-DALLOCATOR=$(ALLOCATOR) \
		-DCEPH_BRANCH=$(CEPH_BRANCH) \
		-DEXTERNAL_ALWAYS_BUILD=$(ALWAYS_BUILD) \
		-DEXTERNAL_DOWNLOAD_DIR=$(DOWNLOADDIR)/$(1) \
		-DCMAKE_TOOLCHAIN_FILE=$(CURDIR)/toolchain/$(1).cmake \
		-DEXTERNAL_LOGGING=$(EXTERNAL_LOGGING) \
		-DEXTERNAL_CCACHE=$(EXTERNAL_CCACHE) \
		-DCMAKE_BUILD_TYPE=$(BUILD_TYPE) \
		-DCMAKE_POSITION_INDEPENDENT_CODE=ON

	@+$(MAKE) -C $(BUILDDIR)/$(1)
endef

$(foreach p,$(PLATFORMS),$(eval $(call build-platform,$(p))))

build: build.$(HOST)

cross: $(foreach p,$(PLATFORMS), build.$(p) )

clean:
	@rm -fr $(BUILDDIR)

distclean: clean
	 @rm -fr $(DOWNLOADDIR)

.PHONY: build cross clean distclean

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build         Build project for host platform.'
	@echo '    cross         Build project for all platforms.'
	@echo '    clean         Remove all files that are created '
	@echo '                  by building.'
	@echo '    distclean     Remove all files that are created '
	@echo '                  by building or configuring.'
	@echo '    help          Show this help info.'
	@echo ''
	@echo 'Options:'
	@echo '    ALLOCATOR     The memory allocator to use.'
	@echo '                  Options are libc, jemalloc, tcmalloc, and'
	@echo '                  tcmalloc_minimal (default).'
	@echo '    ALWAYS_BUILD  Set to 1 to always build ceph. The'
	@echo '                  default is 1.'
	@echo '    CEPH_BRANCH   The ceph branch to build from. Can be set'
	@echo '                  to kraken, or luminous.'
	@echo '    BUILD_TYPE    The build type. Debug produces non-'
	@echo '                  optimized builds. Release produces fully'
	@echo '                  optimized builds. RelWithDebInfo produces'
	@echo '                  optimized builds with debug information.'
	@echo '                  The default is RelWithDebInfo.'
	@echo '    CCACHE        Set to 1 to use ccache. The default is 1.'
	@echo '    V             Set to 1 enable verbose build. Default is 0.'
