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

# When running the tag pipeline or release build we always want to add the alpha, beta, or rc suffix
# if provided. Otherwise, it's a master build where we don't want to apply the suffix.
ifneq ($(TAG_WITH_SUFFIX),true)
override VERSION := $(shell echo "$(VERSION)" | sed -e 's/-alpha.0//' -e 's/-beta.0//' -e 's/-rc.0//')
endif

DOCS_PREFIX ?= docs/rook

ifeq ($(shell echo $(BRANCH_NAME)),master)
DOCS_VERSION := latest
else
DOCS_VERSION := $(shell echo $(BRANCH_NAME) | sed -E "s/^release\-([0-9]+)\.([0-9]+)$$/v\1.\2/g")
endif

ifeq ($(shell echo $(BRANCH_NAME)),$(shell $(ROOT_DIR)/build/release/latest_release_branch.sh))
DOCS_VERSION_ALIAS := latest-release
else
DOCS_VERSION_ALIAS :=
endif

DOCS_GIT_REPO ?= git@github.com:rook/rook.github.io.git

ifeq ($(origin BRANCH_NAME), undefined)
BRANCH_NAME := $(shell git branch | grep \* | cut -d ' ' -f2)
endif

ifeq ($(COMMIT_HASH),)
override COMMIT_HASH := $(shell git rev-parse HEAD)
endif

PLATFORMS ?= $(ALL_PLATFORMS)

# "helm" flavor is for pushing to s3/https endpoint
# "charts" flavor is for pushing to oci endpoint
FLAVORS ?= output images docs helm charts

DOCKER_REGISTRY ?= docker.io/rook
QUAY_REGISTRY ?= quay.io/rook
GHCR_REGISTRY ?= ghcr.io/rook
ifeq ($(TAGGED_RELEASE),false)
# Publish always to dockerhub, including master and interim builds
REGISTRIES = $(DOCKER_REGISTRY)
else
# Publish the images to all registries for tagged releases
REGISTRIES = $(DOCKER_REGISTRY) $(QUAY_REGISTRY) $(GHCR_REGISTRY)
endif
IMAGE_ARCHS := $(subst linux_,,$(filter linux_%,$(PLATFORMS)))
IMAGE_PLATFORMS := $(subst _,/,$(subst $(SPACE),$(COMMA),$(filter linux_%,$(PLATFORMS))))
IMAGE_PLATFORMS_COMMA := $(shell echo "$(IMAGE_PLATFORMS)" | sed 's/ /,/g' | sed 's/.$$//')

S3_BUCKET ?= rook.releases
S3_SYNC := aws s3 sync --only-show-errors
S3_SYNC_DEL := aws s3 sync --only-show-errors --delete

# ====================================================================================
# tools

MANIFEST_TOOL_VERSION=v1.0.2
MANIFEST_TOOL := $(TOOLS_HOST_DIR)/manifest-tool-$(MANIFEST_TOOL_VERSION)

$(MANIFEST_TOOL):
	@echo === installing manifest-tool
	mkdir -p $(TOOLS_HOST_DIR)
	curl -sL https://github.com/estesp/manifest-tool/releases/download/$(MANIFEST_TOOL_VERSION)/manifest-tool-$(shell go env GOHOSTOS)-$(GOHOSTARCH) > $@
	chmod +x $@

# ====================================================================================
# Targets

build: $(addprefix build.,$(FLAVORS)) ;
publish: $(addprefix publish.,$(FLAVORS)) ;
promote: $(addprefix promote.,$(FLAVORS)) ;

# catch all for unimplemented targets / flavors
%: ; @:

# ====================================================================================
# docs

deps.docs:
	pip3 install -r requirements_docs.txt

build.docs: deps.docs
	# Build the docs once before they are published to ensure they are correctly building
	cd $(ROOT_DIR) && mkdocs build --strict

publish.docs:
	if ! git remote show docs > /dev/null 2>&1; then git remote add docs $(DOCS_GIT_REPO); fi
	# If we are running in GitHub actions, remove the extraheader so our custom GitHub API token is used
	if test "$$GITHUB_ACTIONS" = "true"; then git config --unset 'http.https://github.com/.extraheader'; fi
	git fetch -u docs gh-pages
	# Switch to root of repo and then run the build and deploy of the documentation
	cd $(ROOT_DIR) && mike deploy --remote docs --push --branch gh-pages --update-aliases --deploy-prefix $(DOCS_PREFIX) $(DOCS_VERSION) $(DOCS_VERSION_ALIAS)

# ====================================================================================
# helm
HELM_CHANNEL = release
HELM_TEMP := $(shell mktemp -d)
HELM_URL := $(HELM_BASE_URL)/$(HELM_CHANNEL)

promote.helm: $(HELM)
#	copy existing charts to a temp dir, then combine with new charts, reindex, and upload
	$(S3_SYNC) s3://$(HELM_S3_BUCKET)/$(HELM_CHANNEL) $(HELM_TEMP)
	$(S3_SYNC) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION)/charts $(HELM_TEMP)
	$(HELM) repo index --url $(HELM_URL) --merge $(HELM_TEMP)/index.yaml $(HELM_TEMP)
	$(S3_SYNC_DEL) $(HELM_TEMP) s3://$(HELM_S3_BUCKET)/$(HELM_CHANNEL)
	rm -fr $(HELM_TEMP)

# set the helm cart version number.
# you should not need to do this for the majority of scenarios.
ifeq ($(origin HELM_CHART_VERSION), undefined)
# this changes the third '.' to a '-':
#   example v1.16.0.371.g4aec70601 -> v1.16.0-371.g4aec70601
# as helm has strict rules on allowable versions to comply with SemVer2
HELM_CHART_VERSION := $(shell echo "$(VERSION)" | sed 's/\./-/3' )
endif
export HELM_CHART_VERSION

# we need to re-package the charts with the HELM_CHART_VERSION,
# as there is a difference with the helm charts that was
# created during the release scripts
define chart.build
build.chart.$(1):
	@echo === helm package $(1)
	mkdir -p $(HELM_OUTPUT_DIR)
	$(HELM) package --version $(HELM_CHART_VERSION) --app-version $(VERSION) $(HELM_CHARTS_DIR)/$(1) --destination $(HELM_OUTPUT_DIR)
build.all.charts: build.chart.$(1)
endef
$(foreach c,$(HELM_CHARTS),$(eval $(call chart.build,$(c))))

define chart.targets
promote.chart.$(1).$(2):
	@echo === helm push $(2) to $(1)
	$(HELM) push $(HELM_OUTPUT_DIR)/$(2)-$(HELM_CHART_VERSION).tgz oci://$(1)
promote.all.charts: promote.chart.$(1).$(2)
endef
$(foreach r,$(REGISTRIES), $(foreach c,$(HELM_CHARTS),$(eval $(call chart.targets,$(r),$(c)))))

# ====================================================================================
# output

publish.output:
	$(S3_SYNC_DEL) $(OUTPUT_DIR) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION)
promote.output:
	$(S3_SYNC_DEL) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(HELM_CHANNEL)/$(VERSION)
	$(S3_SYNC_DEL) s3://$(S3_BUCKET)/build/$(BRANCH_NAME)/$(VERSION) s3://$(S3_BUCKET)/$(HELM_CHANNEL)/current

# ====================================================================================
# images

# 1: registry 2: arch
define repo.targets
build.image.$(1).$(2):
	$(DOCKERCMD) tag $(BUILD_REGISTRY)/ceph-$(2) $(1)/ceph-$(2):$(VERSION)
	$(DOCKERCMD) tag $(BUILD_REGISTRY)/ceph-$(2) $(1)/ceph-$(2):$(BRANCH_NAME)
	@# Save image as _output/images/linux_<arch>/ceph.tar.gz (no builds for darwin or windows)
	mkdir -p $(OUTPUT_DIR)/images/linux_$(2)
	$(DOCKERCMD) save $(BUILD_REGISTRY)/ceph-$(2) | gzip -c > $(OUTPUT_DIR)/images/linux_$(2)/ceph.tar.gz
build.all.images: build.image.$(1).$(2)
publish.image.$(1).$(2):
	@$(DOCKERCMD) push $(1)/ceph-$(2):$(VERSION)
	@$(DOCKERCMD) push $(1)/ceph-$(2):$(BRANCH_NAME)

publish.all.images.$(1): publish.image.$(1).$(2)
endef
$(foreach r,$(REGISTRIES), $(foreach a,$(IMAGE_ARCHS),$(eval $(call repo.targets,$(r),$(a)))))

define repo.manifest.targets
publish.manifest.image.$(1): publish.all.images.$(1) $(MANIFEST_TOOL)
	$(MANIFEST_TOOL) push from-args --platforms $(IMAGE_PLATFORMS_COMMA) --template $(1)/ceph-ARCH:$(VERSION) --target $(1)/ceph:$(VERSION)
	$(MANIFEST_TOOL) push from-args --platforms $(IMAGE_PLATFORMS_COMMA) --template $(1)/ceph-ARCH:$(BRANCH_NAME) --target $(1)/ceph:$(BRANCH_NAME)

publish.images: publish.manifest.image.$(1)
endef
$(foreach r,$(REGISTRIES),$(eval $(call repo.manifest.targets,$(r))))


build.images: build.all.images
build.charts: build.all.charts
promote.charts: promote.all.charts

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build        Build all release artifacts.'
	@echo '    publish      Publish all release artifacts.'
	@echo '    promote      Publish the helm charts.'
	@echo ''
	@echo 'Options:'
	@echo '    VERSION      Sets the release version.'
	@echo '    BRANCH_NAME  Name of the branch we are releasing from.'
	@echo '    PLATFORMS    The supported platforms to build when running.'
	@echo '                 the build.all target. The default is'
	@echo '                 all supported platforms'
