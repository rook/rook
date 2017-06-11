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

include ../image.mk

# the git repo and branch to build from
CEPH_GIT_REPO ?= https://github.com/rook/ceph
CEPH_GIT_BRANCH ?= rook-luminous

# the git commit hash to build
ifeq ($(origin CEPH_GIT_COMMIT), undefined)
CEPH_GIT_COMMIT := $(shell cat commit_hash)
endif

# set to release or relwithdebinfo (make sure its lower case)
CEPH_BUILD_TYPE ?= release

# set to tcmalloc, tcmalloc_minimal, jemalloc or libc
CEPH_ALLOCATOR ?= tcmalloc
ifeq ($(filter libc tcmalloc tcmalloc_minimal jemalloc, $(CEPH_ALLOCATOR)),)
$(error unsupported memory allocator $(CEPH_ALLOCATOR))
endif

CEPH_CONFIG ?= $(GOARCH)-$(CEPH_ALLOCATOR)-$(CEPH_BUILD_TYPE)

CEPH_TARBALL = ceph-install.tar

CCACHE_DIR_HOST ?= $(HOME)/.ccache

CEPH_BUILDER_IMAGE := $(BUILD_REGISTRY)/ceph-builder-$(CEPH_CONFIG)
CEPH_BUILDER_BASEIMAGE := $(BUILD_REGISTRY)/cross-gnu-$(GOHOSTARCH)

CEPH_IMAGE := $(BUILD_REGISTRY)/ceph-$(CEPH_CONFIG)-$$(docker images -q $(CEPH_BUILDER_IMAGE))

IMAGES = $(CEPH_BUILDER_IMAGE) $(CEPH_IMAGE)
CACHE_IMAGES = $(IMAGES)


TEMP := $(shell mktemp -d)

# ====================================================================================
# Targets

build.images:
	@echo === docker build $(CEPH_BUILDER_IMAGE)
	@cp -a . $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(CEPH_BUILDER_BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) \
		--build-arg ARCH=$(DEBIAN_ARCH) \
		--build-arg CEPH_GIT_COMMIT=$(CEPH_GIT_COMMIT) \
		--build-arg CEPH_GIT_REPO=$(CEPH_GIT_REPO) \
		--build-arg CEPH_GIT_BRANCH=$(CEPH_GIT_BRANCH) \
		--build-arg CEPH_ALLOCATOR=$(CEPH_ALLOCATOR) \
		--build-arg CEPH_BUILD_TYPE=$(CEPH_BUILD_TYPE) \
		--build-arg CROSS_TRIPLE=$(CROSS_TRIPLE) \
		-t $(CEPH_BUILDER_IMAGE) \
		$(TEMP)
#	lookup the rootfs image from the cache or make it if its not there
	@$(MAKE) cache.lookup LOOKUP_IMAGE=$(CEPH_IMAGE) MISS_TARGET=ceph

ceph:
	@echo === building ceph from $(CEPH_BUILDER_IMAGE)
	@docker run \
		-i $(RUN_ARGS) \
		--cidfile $(TEMP)/cid \
		-v $(CCACHE_DIR_HOST):/root/.ccache \
		$(CEPH_BUILDER_IMAGE)
	@docker cp `cat $(TEMP)/cid`:/build/$(CEPH_TARBALL) $(TEMP)/$(CEPH_TARBALL)
	@docker rm -f `cat $(TEMP)/cid`
	@docker import $(TEMP)/$(CEPH_TARBALL) $(CEPH_IMAGE)
	@rm -fr $(TEMP)

%/$(CEPH_TARBALL): build
	@CID=`docker create $(CEPH_IMAGE) /dev/null` &&\
	  docker export $$CID > $@ &&\
	  docker rm -f $$CID
