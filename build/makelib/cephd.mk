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
# Makefile helper functions for building embedded ceph

# TODO: support for profiling in ceph
# TODO: support for tracing in ceph
# TODO: jemalloc and static linking are currently broken due to https://github.com/jemalloc/jemalloc/issues/442
# TODO: remove leveldb from ceph
# TODO: rocksdb build_version always building
# TODO: rocksdb should not build from source tree

# ====================================================================================
# Configuration

CEPHD_ALLOCATOR ?= tcmalloc_minimal
CEPHD_CCACHE ?= 1
CEPHD_DEBUG ?= 0
CEPHD_DIR ?= ceph
CEPHD_BUILD_DIR ?= ceph/build
CEPHD_PLATFORM ?= linux_amd64

# ====================================================================================
# Configuration

CEPHD_CMAKE += \
	-DWITH_EMBEDDED=ON \
	-DALLOCATOR=$(CEPHD_ALLOCATOR) \
	-DWITH_FUSE=OFF \
	-DWITH_NSS=OFF \
	-DUSE_CRYPTOPP=ON \
	-DWITH_LEVELDB=OFF \
	-DWITH_XFS=OFF \
	-DWITH_OPENLDAP=OFF \
	-DWITH_MANPAGE=OFF \
	-DWITH_PROFILER=OFF \
	-DWITH_LTTNG=OFF \
	-DWITH_MGR=OFF \
	-DWITH_PYTHON3=OFF

ifeq ($(CEPHD_CCACHE),1)
CEPHD_CMAKE += -DWITH_CCACHE=ON
endif

# ====================================================================================
# Targets

.PHONY: cephd.config
cephd.config:
	@mkdir -p $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)
	@echo "$(CEPHD_CMAKE)" > $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/cephd.cmake.new
	@if test ! -f $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/cephd.cmake || ! diff $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/cephd.cmake.new $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/cephd.cmake > /dev/null; then \
		cd $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM) && cmake $(CEPHD_CMAKE) -DCMAKE_TOOLCHAIN_FILE=$(abspath build/container/external/toolchain/gcc.$(CEPHD_PLATFORM).cmake) $(abspath ceph); \
		echo "$(CEPHD_CMAKE)" > cephd.cmake; \
	fi
	@rm $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/cephd.cmake.new

.PHONY: cephd.build
cephd.build: cephd.config
	@cd $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM) && $(MAKE) cephd
ifeq ($(CEPHD_DEBUG),0)
	@$(STRIP) -pd $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/lib/libcephd.a
endif
	@touch -c --reference=$(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/lib/libcephd.a $(CEPHD_TOUCH_ON_BUILD)

.PHONY: cephd.clean
cephd.clean:
	@rm -rf $(CEPHD_BUILD_DIR)
