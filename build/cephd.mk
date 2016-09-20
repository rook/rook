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
	-DCMAKE_TARGET_MESSAGES=OFF \

ifeq ($(CEPHD_CCACHE),1)
CEPHD_CMAKE += -DWITH_CCACHE=ON
endif

# poor man's dependencies. this is a notriously hard problem with recursive make. the
# compromise here is good enough. For release builds we always build clean so this
# not be an issue. For developer build, the worst that can happen is we miss a changed
# file and libcephd.a does not build. rm libcephd.a or touching ceph/build/*/Makefile
# would fix that.
CEPHD_SOURCES := $(shell find ceph/src -name .git -prune -o -type f | sed 's/ /\\ /g')

# ====================================================================================
# Targets

# build cephd for the specified platform
# $1 platform name (for example castlectl)
define cephd-build

$(CEPHD_BUILD_DIR)/$(1)/Makefile: Makefile
	@mkdir -p $$(@D)
	@echo "$(CEPHD_CMAKE)" > $$(@D)/cephd.cmake.new
	@if test ! -f $$(@D)/cephd.cmake || ! diff $$(@D)/cephd.cmake.new $$(@D)/cephd.cmake > /dev/null; then \
		echo "====== Configuring cephd $(1)"; \
		cd $$(@D) && cmake $(CEPHD_CMAKE) -DCMAKE_TOOLCHAIN_FILE=../../../build/cross-build/toolchain.$(1).cmake ../..; \
		echo "$(CEPHD_CMAKE)" > cephd.cmake; \
	fi
	@rm $$(@D)/cephd.cmake.new
	
$(CEPHD_BUILD_DIR)/$(1)/lib/libcephd.a: $(CEPHD_BUILD_DIR)/$(1)/Makefile $(CEPHD_SOURCES)
	@echo "====== Building cephd $(1)"
	@cd $(CEPHD_BUILD_DIR)/$(1) && $$(MAKE) cephd
ifeq ($(CEPHD_DEBUG),0)
	@strip -pS $$@
endif

.PHONY: cephd.clean.$(1)
cephd.clean.$(1): cephd.clean.rocksdb
	@echo "====== Running make clean on cephd $(1)"
	@[ -d $(CEPHD_BUILD_DIR)/$(1) ] && cd $(CEPHD_BUILD_DIR)/$(1) && $(MAKE) clean
endef

# BUGBUG: rocksdb builds out of the source tree which means arm64 and amd64
# binaries will overwrite each other
.PHONY: cephd.clean.rocksdb
cephd.clean.rocksdb:
	@[ -d ceph/src/rocksdb ] && cd ceph/src/rocksdb && $(MAKE) clean > /dev/null

.PHONY: cephd.clean
cephd.cleanall: cephd.clean.rocksdb
	@rm -rf $(CEPHD_BUILD_DIR)
