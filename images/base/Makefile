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

# the final base image
IMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

# the full OS image from which we generate a smaller rootfs
ROOTFS_BUILDER_IMAGE := $(BUILD_REGISTRY)/rootfs-builder-$(GOARCH)

# the smaller rootfs that will be cached in docker as an image. we use
# the image id of the builder in the image name to chain them, and ensure
# that when the rootfs builder images that this one is invalidated.
ROOTFS_IMAGE := $(BUILD_REGISTRY)/rootfs-$(GOARCH)-$$(docker images -q $(ROOTFS_BUILDER_IMAGE))

IMAGES = $(IMAGE) $(ROOTFS_BUILDER_IMAGE) $(ROOTFS_IMAGE)
CACHE_IMAGES = $(IMAGES)

TEMP := $(shell mktemp -d)

rootfs-build:
	@echo === docker build $(ROOTFS_BUILDER_IMAGE)
	@cp Dockerfile.rootfs-builder $(TEMP)/Dockerfile
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(OSBASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_BASE_ARGS) -t $(ROOTFS_BUILDER_IMAGE) $(TEMP)
#	lookup the rootfs image from the cache or make it if its not there
	@$(MAKE) cache.lookup LOOKUP_IMAGE=$(ROOTFS_IMAGE) MISS_TARGET=rootfs

rootfs:
	@echo === docker build $(ROOTFS_IMAGE)
	@CID=`docker create $(ROOTFS_BUILDER_IMAGE) /bin/bash` &&\
	  docker export $$CID > $(TEMP)/rootfs.tar &&\
	  docker rm -f $$CID
	@cp Dockerfile.rootfs $(TEMP)/Dockerfile
	@docker build $(BUILD_ARGS) -t $(ROOTFS_IMAGE) $(TEMP)
	@rm -fr $(TEMP)

build.images: rootfs-build
	@echo === docker build $(IMAGE)
	@cp Dockerfile $(TEMP)
	@cd $(TEMP) && $(SED_CMD) "s|BASEIMAGE|$(ROOTFS_IMAGE)|g" Dockerfile
	@docker build $(BUILD_ARGS) --build-arg ARCH=$(DEBIAN_ARCH) -t $(IMAGE) $(TEMP)
	@rm -fr $(TEMP)
