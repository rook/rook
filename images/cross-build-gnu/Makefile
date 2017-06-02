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

# cross-build only supported on amd64
override GOARCH := amd64
include ../image.mk

# ====================================================================================
# Build Options

REPO ?= cross-build-gnu
IMAGE = $(BUILD_REGISTRY)/$(REPO)
BASEIMAGE = $(BUILD_REGISTRY)/base-$(GOARCH)

# ====================================================================================
# Build image

TEMP := $(shell mktemp -d)
CLEAN_IMAGES := $(IMAGE)

build:
	@echo === docker build $(IMAGE)
	@cp -a . $(TEMP)
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@docker build $(BUILD_ARGS) -t $(IMAGE) $(TEMP)
	@rm -fr $(TEMP)

clean: clean.images