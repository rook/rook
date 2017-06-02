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

.PHONY: all build clean
all: build

include ../image.mk

# ====================================================================================
# Build Options

REPO ?= base
IMAGE = $(BUILD_REGISTRY)/$(REPO)-$(GOARCH)

# ====================================================================================
# Build rootfs

ROOTFS_BUILD_IMAGE := $(BUILD_REGISTRY)/rootfs-build-$(GOARCH)
ROOTFS_IMAGE := $(HOST_REGISTRY)/rootfs-$(GOARCH):$$(docker images -q $(ROOTFS_BUILD_IMAGE))
ROOTFS_FILENAME = rootfs.tar

TEMP := $(shell mktemp -d)
CLEAN_IMAGES += $(IMAGE) $(ROOTFS_BUILD_IMAGE)

# NOTE: we check if docker build actually built anything (due to changes)
# by comparing the image id before and after calling docker build. If there
# were changes then we proceed with extracting the rootfs and creating the
# final image. This prevents uncessary builds of containers that use this base image.

rootfs-build:
	@echo === docker build $(ROOTFS_BUILD_IMAGE)
	@cp Dockerfile.rootfs-build $(TEMP)/Dockerfile
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(OSBASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_BASE_ARGS) -t $(ROOTFS_BUILD_IMAGE) $(TEMP)
	@[ -n "$$(docker images -q $(ROOTFS_IMAGE))" ] || $(MAKE) rootfs

rootfs:
	@echo === docker build $(ROOTFS_IMAGE)
	@CID=`docker create $(ROOTFS_BUILD_IMAGE) /bin/bash` &&\
	  docker export $$CID > $(TEMP)/$(ROOTFS_FILENAME) &&\
	  docker rm -f $$CID

	@cp Dockerfile.rootfs $(TEMP)/Dockerfile
	@docker build $(BUILD_ARGS) -t $(ROOTFS_IMAGE) $(TEMP)
	@rm -fr $(TEMP)

build: rootfs-build
	@echo === docker build $(IMAGE)
	@cp Dockerfile $(TEMP)
	@cd $(TEMP) && $(SED_CMD) "s|BASEIMAGE|$(ROOTFS_IMAGE)|g" Dockerfile
	@docker build $(BUILD_ARGS) -t $(IMAGE) $(TEMP)
	@rm -fr $(TEMP)

clean: clean.images