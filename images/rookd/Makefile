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

ROOKD_IMAGE = $(BUILD_REGISTRY)/rookd-$(GOARCH)
ROOKD_BASE_IMAGE = $(BUILD_REGISTRY)/rookd-base-$(GOARCH)
IMAGES = $(ROOKD_IMAGE) $(ROOKD_BASE_IMAGE)
CACHE_IMAGES = $(ROOKD_BASE_IMAGE)

BASEIMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Ceph

CEPH_BUILD_TYPE ?= release
CEPH_ALLOCATOR ?= tcmalloc

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
	@echo === building ceph for $(ROOKD_IMAGE)
	@$(MAKE) -C ../ceph GOARCH=$(GOARCH) CEPH_ALLOCATOR=$(CEPH_ALLOCATOR) CEPH_BUILD_TYPE=$(CEPH_BUILD_TYPE) $(CEPH_TARBALL)

# ====================================================================================
# Build Rook

rookd-base-image: $(CEPH_TARBALL)
	@echo === docker build $(ROOKD_BASE_IMAGE)
	@cp Dockerfile.base $(TEMP)/Dockerfile
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) --build-arg EXTRALIBS=$(EXTRA_LIBS) -t $(ROOKD_BASE_IMAGE) $(TEMP)

# since this is a leaf image avoid leaving around a lot of dangling images
# by removing the last build of the final rookd image
OLD_IMAGE_ID := $(shell docker images -q $(ROOKD_IMAGE))

build.images: rookd-base-image
	@echo === docker build $(ROOKD_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp ../../bin/linux_$(GOARCH)/rookd $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(ROOKD_BASE_IMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) -t $(ROOKD_IMAGE) $(TEMP)
	@[ -z "$(OLD_IMAGE_ID)" ] || docker rmi $(OLD_IMAGE_ID)
	@rm -fr $(TEMP)
