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

CHANNEL ?= master
ifeq ($(filter master alpha beta stable,$(CHANNEL)),)
$(error invalid channel $(CHANNEL))
endif

PLATFORMS ?= $(ALL_PLATFORMS)
FLAVORS ?= binaries images docs
TEMP_DIR := $(shell mktemp -d)

DOCKER_REGISTRY ?= rook
QUAY_REGISTRY ?= quay.io/rook
REGISTRIES ?= $(DOCKER_REGISTRY) $(QUAY_REGISTRY)
IMAGES ?= rook toolbox
IMAGE_ARCHS := $(subst linux_,,$(filter linux_%,$(PLATFORMS)))
IMAGE_PLATFORMS := $(subst _,/,$(subst $(SPACE),$(COMMA),$(filter linux_%,$(PLATFORMS))))

S3_BUCKET ?= rook-release
S3_CP := aws s3 cp --only-show-errors
S3_SYNC := aws s3 sync --only-show-errors --delete
S3_CURRENT_BUCKET := s3://$(S3_BUCKET)/$(CHANNEL)/$(VERSION)

# TODO: change from ilovett to rook
DOCS_REPO ?= https://github.com/ilovett/rook.github.io.git
DOCS_BRANCH ?= master
DOCS_WORK_DIR := $(WORK_DIR)/rook.github.io
DOCS_VERSION_DIR= $(DOCS_WORK_DIR)/docs/rook/$(VERSION)

# ====================================================================================
# tools

MANIFEST_TOOL_VERSION=v0.5.0
MANIFEST_TOOL := $(TOOLS_HOST_DIR)/manifest-tool-$(MANIFEST_TOOL_VERSION)

$(MANIFEST_TOOL):
	@echo === installing manifest-tool
	@curl -sL https://github.com/estesp/manifest-tool/releases/download/v0.5.0/manifest-tool-$(GOHOSTOS)-$(GOHOSTARCH) > $@
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
# binary files

publish.binaries.%:
	@$(S3_CP) $(OUTPUT_DIR)/$*/rookctl$(if $(filter windows_%,$*),.exe) $(S3_CURRENT_BUCKET)/$*/rookctl$(if $(filter windows_%,$*),.exe)
publish.binaries: $(addprefix publish.binaries.,$(PLATFORMS))
	@echo "$(VERSION)" > $(TEMP_DIR)/version;
	@$(S3_CP) "$(TEMP_DIR)/version" $(S3_CURRENT_BUCKET)/version
promote.binaries:
	@$(S3_SYNC) s3://$(S3_BUCKET)/master/$(VERSION) s3://$(S3_BUCKET)/$(CHANNEL)/current

# ====================================================================================
# docs

publish.docs:
	@rm -rf $(DOCS_WORK_DIR) && mkdir -p $(DOCS_WORK_DIR)
	@git clone --depth=1 -b $(DOCS_BRANCH) $(DOCS_REPO) $(DOCS_WORK_DIR)
	@rm -rf $(DOCS_VERSION_DIR) && mkdir -p $(DOCS_VERSION_DIR)
	@cp -r ../../Documentation/ $(DOCS_VERSION_DIR)/
	@pushd $(DOCS_WORK_DIR) >/dev/null && \
		npm install && \
		node ./prepare.js && \
		git add . && \
		git commit -m "docs snapshot for channel $(VERSION)" && \
		git push -u origin $(DOCS_BRANCH) && \
		popd >/dev/null

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
	@echo '    CHANNEL      Sets the release channel. Can be set to master,'
	@echo '                 alpha, beta, or stable. Default is not set.'
	@echo '    PLATFORMS    The supported platforms to build when running.'
	@echo '                 the build.all target. The default is'
	@echo '                 all supported platforms'
