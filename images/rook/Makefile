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

# ====================================================================================
# Image Build Options

ROOK_IMAGE = $(BUILD_REGISTRY)/rook-$(GOARCH)
ROOK_BASE_IMAGE = $(BUILD_REGISTRY)/rook-base-$(GOARCH)
IMAGES = $(ROOK_IMAGE) $(ROOK_BASE_IMAGE)
CACHE_IMAGES = $(ROOK_BASE_IMAGE)

BASEIMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Ceph

CEPH_BUILD_TYPE ?= release

ifeq ($(GOARCH),amd64)
CEPH_ALLOCATOR ?= tcmalloc
else
CEPH_ALLOCATOR ?= libc
endif

ifeq ($(CEPH_ALLOCATOR),tcmalloc)
EXTRA_LIBS += libgoogle-perftools4
endif
ifeq ($(CEPH_ALLOCATOR),tcmalloc_minimal)
EXTRA_LIBS += libtcmalloc_minimal4
endif
ifeq ($(CEPH_ALLOCATOR),jemalloc)
EXTRA_LIBS += libjemalloc1
endif

CEPH_TARBALL_FILENAME = ceph-install.tar
CEPH_TARBALL = $(TEMP)/$(CEPH_TARBALL_FILENAME)

$(CEPH_TARBALL):
	@echo === building ceph for $(ROOK_IMAGE)
	@$(MAKE) -C ../ceph GOARCH=$(GOARCH) CEPH_ALLOCATOR=$(CEPH_ALLOCATOR) CEPH_BUILD_TYPE=$(CEPH_BUILD_TYPE) $(CEPH_TARBALL)

# ====================================================================================
# Build Rook

rook-base-image: $(CEPH_TARBALL)
	@echo === docker build $(ROOK_BASE_IMAGE)
	@cp Dockerfile.base $(TEMP)/Dockerfile
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) --build-arg EXTRALIBS=$(EXTRA_LIBS) -t $(ROOK_BASE_IMAGE) $(TEMP)

# since this is a leaf image we avoid leaving around a lot of dangling images
# by removing the last build of the final rook image
OLD_IMAGE_ID := $(shell docker images -q $(ROOK_IMAGE))
CURRENT_IMAGE_ID := $$(docker images -q $(ROOK_IMAGE))
IMAGE_FILENAME := $(IMAGE_OUTPUT_DIR)/rook.tar.gz

do.build: rook-base-image
	@echo === docker build $(ROOK_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rookflex $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(ROOK_BASE_IMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) -t $(ROOK_IMAGE) $(TEMP)
	@[ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] && [ -n "$(OLD_IMAGE_ID)" ] && docker rmi $(OLD_IMAGE_ID) || true
	@if [ ! -e "$(IMAGE_FILENAME)" ] || [ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] || [ -n "$(OLD_IMAGE_ID)" ]; then \
		echo === saving image $(ROOK_IMAGE); \
		mkdir -p $(IMAGE_OUTPUT_DIR); \
		docker save $(ROOK_IMAGE) | gzip -c > $(IMAGE_FILENAME); \
	fi
	@rm -fr $(TEMP)
