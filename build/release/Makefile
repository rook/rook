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
include ../makelib/helm.mk

# ====================================================================================
# Options

CHANNEL ?= master
ifeq ($(filter master alpha beta stable,$(CHANNEL)),)
$(error invalid channel $(CHANNEL))
endif

DOCS_VERSION := $(shell echo $(BRANCH_NAME) | sed -E "s/^release\-([0-9]+)\.([0-9]+)$$/v\1.\2/g")
DOCS_DIR ?= $(ROOT_DIR)/Documentation
DOCS_WORK_DIR := $(WORK_DIR)/rook.github.io
DOCS_VERSION_DIR := $(DOCS_WORK_DIR)/docs/rook/$(DOCS_VERSION)

ifdef GIT_API_TOKEN
DOCS_GIT_REPO := https://$(GIT_API_TOKEN)@github.com/rook/rook.github.io.git
else
DOCS_GIT_REPO := git@github.com:rook/rook.github.io.git
endif

ifeq ($(origin BRANCH_NAME), undefined)
BRANCH_NAME := $(shell git branch | grep \* | cut -d ' ' -f2)
endif

ifeq ($(COMMIT_HASH),)
override COMMIT_HASH := $(shell git rev-parse HEAD)
endif

REMOTE_NAME ?= origin

PLATFORMS ?= $(ALL_PLATFORMS)

ifneq ($(filter master release-%,$(BRANCH_NAME)),)
FLAVORS ?= output images docs helm
else
FLAVORS ?= output
override BRANCH_NAME := pr/$(BRANCH_NAME)
endif

DOCKER_REGISTRY ?= rook
QUAY_REGISTRY ?= quay.io/rook
REGISTRIES ?= $(DOCKER_REGISTRY) $(QUAY_REGISTRY)
IMAGES ?= ceph toolbox
IMAGE_ARCHS := $(subst linux_,,$(filter linux_%,$(PLATFORMS)))
IMAGE_PLATFORMS := $(subst _,/,$(subst $(SPACE),$(COMMA),$(filter linux_%,$(PLATFORMS))))

S3_BUCKET ?= rook.release
S3_CP := aws s3 cp --only-show-errors
S3_SYNC := aws s3 sync --only-show-errors
S3_SYNC_DEL := aws s3 sync --only-show-errors --delete

# ====================================================================================
# tools

MANIFEST_TOOL_VERSION=v0.7.0
MANIFEST_TOOL := $(TOOLS_HOST_DIR)/manifest-tool-$(MANIFEST_TOOL_VERSION)

$(MANIFEST_TOOL):
	@echo === installing manifest-tool
	@mkdir -p $(TOOLS_HOST_DIR)
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
# tagging a release

VERSION_REGEX := ^v\([0-9]*\)[.]\([0-9]*\)[.]\([0-9]*\)$$
VERSION_VALID := $(shell echo "$(VERSION)" | grep -q '$(VERSION_REGEX)' && echo 1 || echo 0)
VERSION_MAJOR := $(shell echo "$(VERSION)" | sed -e 's/$(VERSION_REGEX)/\1/')
VERSION_MINOR := $(shell echo "$(VERSION)" | sed -e 's/$(VERSION_REGEX)/\2/')
VERSION_PATCH := $(shell echo "$(VERSION)" | sed -e 's/$(VERSION_REGEX)/\3/')

tag:
ifneq ($(VERSION_VALID),1)
	$(error invalid version $(VERSION). must be a semantic version with v[Major].[Minor].[Patch] only)
endif
	@echo === tagging commit hash $(COMMIT_HASH) with v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH)
	@git tag -f -m "release $(VERSION)" v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH) $(COMMIT_HASH)
	@git push $(REMOTE_NAME) v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH)
	@set -e; if ! git ls-remote --heads $(REMOTE_NAME) | grep -q refs/heads/release-$(VERSION_MAJOR).$(VERSION_MINOR); then \
		echo === creating new release branch release-$(VERSION_MAJOR).$(VERSION_MINOR) ;\
		git branch -f release-$(VERSION_MAJOR).$(VERSION_MINOR) $(COMMIT_HASH) ;\
		git push $(REMOTE_NAME) release-$(VERSION_MAJOR).$(VERSION_MINOR) ;\
	fi

# ====================================================================================
# docs

build.docs:
	rm -rf $(DOCS_WORK_DIR)
	mkdir -p $(DOCS_WORK_DIR)
	git clone --depth=1 -b master $(DOCS_GIT_REPO) $(DOCS_WORK_DIR)
	rm -rf $(DOCS_VERSION_DIR)
	cp -r $(DOCS_DIR)/ $(DOCS_VERSION_DIR)
	cd $(DOCS_WORK_DIR) && npm install && node build/scripts/preprocess.js

publish.docs:
	$(DOCS_WORK_DIR)/build/publish.sh

# ====================================================================================
# helm

HELM_TEMP := $(shell mktemp -d)
HELM_URL := $(HELM_BASE_URL)/$(CHANNEL)

promote.helm: $(HELM)
#	copy existing charts to a temp dir, the combine with new charts, reindex, and upload
	@$(S3_SYNC) s3://$(HELM_S3_BUCKET)/$(CHANNEL) $(HELM_TEMP)
	@$(S3_SYNC) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION)/charts $(HELM_TEMP)
	@$(HELM) repo index --url $(HELM_URL) $(HELM_TEMP)
	@$(S3_SYNC_DEL) $(HELM_TEMP) s3://$(HELM_S3_BUCKET)/$(CHANNEL)
	@rm -fr $(HELM_TEMP)

# ====================================================================================
# output

publish.output:
	@$(S3_SYNC_DEL) $(OUTPUT_DIR) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION)
promote.output:
	@$(S3_SYNC_DEL) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(CHANNEL)/$(VERSION)
	@$(S3_SYNC_DEL) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(CHANNEL)/current

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
