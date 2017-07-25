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

all: build

include ../makelib/common.mk

# ====================================================================================
# Options

# set the shell to bash in case some environments use sh
SHELL := /bin/bash

CHANNEL ?= master
ifeq ($(filter master alpha beta stable,$(CHANNEL)),)
$(error invalid channel $(CHANNEL))
endif

DOCS_VERSION := $(shell echo $(BRANCH_NAME) | sed -E "s/^release\-([0-9]+)\.([0-9]+).*$$/v\1.\2/g")
DOCS_DIR ?= $(ROOT_DIR)/Documentation
DOCS_WORK_DIR := $(WORK_DIR)/rook.github.io
DOCS_VERSION_DIR= $(DOCS_WORK_DIR)/docs/rook/$(DOCS_VERSION)

ifdef GIT_API_TOKEN
DOCS_GIT_REPO := https://$(GIT_API_TOKEN)@github.com/ilovett/ilovett.github.io.git
else
DOCS_GIT_REPO := git@github.com:ilovett/ilovett.github.io.git
endif

ifeq ($(origin BRANCH_NAME), undefined)
BRANCH_NAME := $(shell git branch | grep \* | cut -d ' ' -f2)
endif

PLATFORMS ?= $(ALL_PLATFORMS)

ifneq ($(filter master release-%,$(BRANCH_NAME)),)
FLAVORS ?= output images docs
else
FLAVORS ?= output
override BRANCH_NAME := pr/$(BRANCH_NAME)
endif

DOCKER_REGISTRY ?= rook
QUAY_REGISTRY ?= quay.io/rook
REGISTRIES ?= $(DOCKER_REGISTRY) $(QUAY_REGISTRY)
IMAGES ?= rook toolbox
IMAGE_ARCHS := $(subst linux_,,$(filter linux_%,$(PLATFORMS)))
IMAGE_PLATFORMS := $(subst _,/,$(subst $(SPACE),$(COMMA),$(filter linux_%,$(PLATFORMS))))

S3_BUCKET ?= rook-release
S3_CP := aws s3 cp --only-show-errors
S3_SYNC := aws s3 sync --only-show-errors --delete


# ====================================================================================
# tools

MANIFEST_TOOL_VERSION=v0.6.0
MANIFEST_TOOL := $(TOOLS_HOST_DIR)/manifest-tool-$(MANIFEST_TOOL_VERSION)

$(MANIFEST_TOOL):
	@echo === installing manifest-tool
	@curl -sL https://github.com/estesp/manifest-tool/releases/download/$(MANIFEST_TOOL_VERSION)/manifest-tool-$(GOHOSTOS)-$(GOHOSTARCH) > $@
	@chmod +x $@

# ====================================================================================
# Targets

build: $(addprefix build.,$(FLAVORS)) ;
publish: $(addprefix publish.,$(FLAVORS)) ;
promote: $(addprefix promote.,$(FLAVORS)) ;
clean: $(addprefix clean.,$(FLAVORS)) ;

# catch all for unimplemented targets / flavors
%: ; @:

# ====================================================================================
# docs

build.docs:

	# checkout a fresh copy of the rook.github.io directory
	@ rm -rf $(DOCS_WORK_DIR)
	@ mkdir -p $(DOCS_WORK_DIR)
	@ git clone --depth=1 -b master $(DOCS_GIT_REPO) $(DOCS_WORK_DIR)

	# replace the version snapshot with the current docs dir
	@ rm -rf $(DOCS_VERSION_DIR)
	@ cp -r $(DOCS_DIR)/ $(DOCS_VERSION_DIR)

	$(DOCS_WORK_DIR)/build/build.sh

publish.docs:

	$(DOCS_WORK_DIR)/build/publish.sh

# ====================================================================================
# output

publish.output:
	@$(S3_SYNC) $(OUTPUT_DIR) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION)
promote.output:
	@$(S3_SYNC) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(CHANNEL)/$(VERSION)
	@$(S3_SYNC) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(CHANNEL)/current

# ====================================================================================
# images

# 1: registry 2: image, 3: arch
define repo.targets
build.image.$(1).$(2).$(3): ; @docker tag $(BUILD_REGISTRY)/$(2)-$(3) $(1)/$(2)-$(3):$(VERSION)
build.all.images: build.image.$(1).$(2).$(3)
publish.image.$(1).$(2).$(3): ; @docker push $(1)/$(2)-$(3):$(VERSION)
publish.all.images: publish.image.$(1).$(2).$(3)
promote.image.$(1).$(2).$(3):
	@docker pull $(1)/$(2)-$(3):$(VERSION)
	@[ "$(CHANNEL)" = "master" ] || docker tag $(1)/$(2)-$(3):$(VERSION) $(1)/$(2)-$(3):$(VERSION)-$(CHANNEL)
	@docker tag $(1)/$(2)-$(3):$(VERSION) $(1)/$(2)-$(3):$(CHANNEL)
	@[ "$(CHANNEL)" = "master" ] || docker push $(1)/$(2)-$(3):$(VERSION)-$(CHANNEL)
	@docker push $(1)/$(2)-$(3):$(CHANNEL)
promote.all.images: promote.image.$(1).$(2).$(3)
clean.image.$(1).$(2).$(3):
	@[ -z "$$$$(docker images -q $(1)/$(2)-$(3):$(VERSION))" ] || docker rmi $(1)/$(2)-$(3):$(VERSION)
	@[ -z "$$$$(docker images -q $(1)/$(2)-$(3):$(VERSION)-$(CHANNEL))" ] || docker rmi $(1)/$(2)-$(3):$(VERSION)-$(CHANNEL)
	@[ -z "$$$$(docker images -q $(1)/$(2)-$(3):$(CHANNEL))" ] || docker rmi $(1)/$(2)-$(3):$(CHANNEL)
clean.all.images: clean.image.$(1).$(2).$(3)
endef
$(foreach r,$(REGISTRIES), $(foreach i,$(IMAGES), $(foreach a,$(IMAGE_ARCHS),$(eval $(call repo.targets,$(r),$(i),$(a))))))

publish.manifest.image.%: publish.all.images $(MANIFEST_TOOL)
	@$(MANIFEST_TOOL) push from-args --platforms $(IMAGE_PLATFORMS) --template $(DOCKER_REGISTRY)/$*-ARCH:$(VERSION) --target $(DOCKER_REGISTRY)/$*:$(VERSION)

promote.manifest.image.%: promote.all.images $(MANIFEST_TOOL)
	@[ "$(CHANNEL)" = "master" ] || $(MANIFEST_TOOL) push from-args --platforms $(IMAGE_PLATFORMS) --template $(DOCKER_REGISTRY)/$*-ARCH:$(VERSION) --target $(DOCKER_REGISTRY)/$*:$(VERSION)-$(CHANNEL)
	@$(MANIFEST_TOOL) push from-args --platforms $(IMAGE_PLATFORMS) --template $(DOCKER_REGISTRY)/$*-ARCH:$(VERSION) --target $(DOCKER_REGISTRY)/$*:$(CHANNEL)

build.images: build.all.images
publish.images: $(addprefix publish.manifest.image.,$(IMAGES))
promote.images: $(addprefix promote.manifest.image.,$(IMAGES))
clean.images: clean.all.images

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build        Build all release artifacts.'
	@echo '    clean        Remove all release artifacts.'
	@echo '    publish      Publish all release artifacts.'
	@echo '    promote      Promote a build to a channel.'
	@echo '    tag          Tag a build for release.'
	@echo ''
	@echo 'Options:'
	@echo '    VERSION      Sets the release version.'
	@echo '    BRANCH_NAME  Name of the branch we're releasing from.'
	@echo '    CHANNEL      Sets the release channel. Can be set to master,'
	@echo '                 alpha, beta, or stable. Default is not set.'
	@echo '    PLATFORMS    The supported platforms to build when running.'
	@echo '                 the build.all target. The default is'
	@echo '                 all supported platforms'
