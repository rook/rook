# Copyright 2018 The Rook Authors. All rights reserved.
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

EDGEFS_IMAGE = $(BUILD_REGISTRY)/edgefs-$(GOARCH)
IMAGES = $(EDGEFS_IMAGE)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Rook EdgeFS

# since this is a leaf image we avoid leaving around a lot of dangling images
# by removing the last build of the final cockroachdb image
OLD_IMAGE_ID := $(shell $(DOCKERCMD) images -q $(EDGEFS_IMAGE))
CURRENT_IMAGE_ID := $$($(DOCKERCMD) images -q $(EDGEFS_IMAGE))
IMAGE_FILENAME := $(IMAGE_OUTPUT_DIR)/edgefs.tar.gz

do.build:
	@echo === docker build $(EDGEFS_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@$(DOCKERCMD) build $(BUILD_ARGS) \
		-t $(EDGEFS_IMAGE) \
		$(TEMP)
	@[ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] && [ -n "$(OLD_IMAGE_ID)" ] && $(DOCKERCMD) rmi $(OLD_IMAGE_ID) || true
	@if [ ! -e "$(IMAGE_FILENAME)" ] || [ "$(OLD_IMAGE_ID)" != "$(CURRENT_IMAGE_ID)" ] || [ -n "$(OLD_IMAGE_ID)" ]; then \
		echo === saving image $(EDGEFS_IMAGE); \
		mkdir -p $(IMAGE_OUTPUT_DIR); \
		$(DOCKERCMD) save $(EDGEFS_IMAGE) | gzip -c > $(IMAGE_FILENAME); \
	fi
	@rm -fr $(TEMP)
