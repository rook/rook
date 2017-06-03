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

.PHONY: all build push
all: build

include ../image.mk

# ====================================================================================
# Options

# the git repo to build from
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
BASEIMAGE := $(BUILD_REGISTRY)/cross-build-gnu
CEPH_BUILD_IMAGE := $(BUILD_REGISTRY)/ceph-build-$(CEPH_CONFIG)
CEPH_IMAGE := $(HOST_REGISTRY)/ceph-$(CEPH_CONFIG):$$(docker images -q $(CEPH_BUILD_IMAGE))
CLEAN_IMAGES = $(CEPH_BUILD_IMAGE)

CEPH_TARBALL = ceph-install.tar

TEMP := $(shell mktemp -d)
CCACHE_DIR_HOST ?= $(HOME)/.ccache

# ====================================================================================
# Targets

build:
	@echo === docker build $(CEPH_BUILD_IMAGE)
	@cp -a . $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) \
		--build-arg ARCH=$(DEBIAN_ARCH) \
		--build-arg CEPH_GIT_COMMIT=$(CEPH_GIT_COMMIT) \
		--build-arg CEPH_GIT_REPO=$(CEPH_GIT_REPO) \
		--build-arg CEPH_GIT_BRANCH=$(CEPH_GIT_BRANCH) \
		--build-arg CEPH_ALLOCATOR=$(CEPH_ALLOCATOR) \
		--build-arg CEPH_BUILD_TYPE=$(CEPH_BUILD_TYPE) \
		--build-arg CROSS_TRIPLE=$(CROSS_TRIPLE) \
		-t $(CEPH_BUILD_IMAGE) \
		$(TEMP)
	@[ -n "$$(docker images -q $(CEPH_IMAGE))" ] || $(MAKE) ceph

ceph:
	@echo === building ceph from $(CEPH_BUILD_IMAGE)
	@docker run \
		-i $(RUN_ARGS) \
		--cidfile $(TEMP)/cid \
		-v $(CCACHE_DIR_HOST):/root/.ccache \
		$(CEPH_BUILD_IMAGE)
	@docker cp `cat $(TEMP)/cid`:/build/$(CEPH_TARBALL) $(TEMP)/$(CEPH_TARBALL)
	@docker rm -f `cat $(TEMP)/cid`
	@docker import $(TEMP)/$(CEPH_TARBALL) $(CEPH_IMAGE)
	@rm -fr $(TEMP)

%/$(CEPH_TARBALL): build
	@CID=`docker create $(CEPH_IMAGE) /dev/null` &&\
	  docker export $$CID > $@ &&\
	  docker rm -f $$CID

clean: clean.images