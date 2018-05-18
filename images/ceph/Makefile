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

CEPH_IMAGE = $(BUILD_REGISTRY)/ceph-$(GOARCH)
CEPH_BASE_IMAGE = $(BUILD_REGISTRY)/ceph-base-$(GOARCH)
IMAGES = $(CEPH_IMAGE) $(CEPH_BASE_IMAGE)
CACHE_IMAGES = $(CEPH_BASE_IMAGE)

BASEIMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Rook

ceph-base-image:
	@echo === docker build $(CEPH_BASE_IMAGE)
	@cp Dockerfile.base $(TEMP)/Dockerfile
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) -t $(CEPH_BASE_IMAGE) $(TEMP)

# since this is a leaf image we avoid leaving around a lot of dangling images
# by removing the last build of the final ceph image
OLD_IMAGE_ID := $(shell docker images -q $(CEPH_IMAGE))
CURRENT_IMAGE_ID := $$(docker images -q $(CEPH_IMAGE))
IMAGE_FILENAME := $(IMAGE_OUTPUT_DIR)/ceph.tar.gz

do.build: ceph-base-image
	@echo === docker build $(CEPH_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rookflex $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(CEPH_BASE_IMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) -t $(CEPH_IMAGE) $(TEMP)
	@[ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] && [ -n "$(OLD_IMAGE_ID)" ] && docker rmi $(OLD_IMAGE_ID) || true
	@if [ ! -e "$(IMAGE_FILENAME)" ] || [ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] || [ -n "$(OLD_IMAGE_ID)" ]; then \
		echo === saving image $(CEPH_IMAGE); \
		mkdir -p $(IMAGE_OUTPUT_DIR); \
		docker save $(CEPH_IMAGE) | gzip -c > $(IMAGE_FILENAME); \
	fi
	@rm -fr $(TEMP)