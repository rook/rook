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
# Image Build Options

REPO ?= rookd
IMAGE = $(REGISTRY)/$(REPO)-$(GOARCH):$(VERSION)
CLEAN_IMAGES = $(IMAGE)

# ====================================================================================
# Ceph Build Options

CEPH_BUILD_TYPE ?= release
CEPH_ALLOCATOR ?= tcmalloc

# ====================================================================================
# Common Build Settings

BASEIMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Ceph

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
	@echo === building ceph for $(IMAGE)
	@$(MAKE) -C ../ceph-build GOARCH=$(GOARCH) CEPH_ALLOCATOR=$(CEPH_ALLOCATOR) CEPH_BUILD_TYPE=$(CEPH_BUILD_TYPE) $(CEPH_TARBALL)

# ====================================================================================
# Build Rook

ROOKD = $(TEMP)/rookd

$(ROOKD):
	@echo === building rookd for $(IMAGE)
	@cp ../../bin/linux_$(GOARCH)/rookd $@

# ====================================================================================
# Build Final Image

EXISTING_IMAGE_ID := $(shell docker images -q $(IMAGE))

build: $(CEPH_TARBALL) $(ROOKD)
	@echo === docker build $(IMAGE)
	@cp Dockerfile $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) --build-arg EXTRALIBS=$(EXTRA_LIBS) -t $(IMAGE) $(TEMP)
	@docker rmi $(EXISTING_IMAGE_ID) > /dev/null 2>&1 || true
	@rm -fr $(TEMP)

push:
	@docker push $(IMAGE)

clean: clean.images