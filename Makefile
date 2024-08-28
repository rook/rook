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

# Linux doesn't guarantee file ordering, so sort the files to make sure order is deterministic.
# And in order to handle file paths with spaces, it's easiest to read the file names into an array.
# Set locale `LC_ALL=C` because different OSes have different sort behavior;
# `C` sorting order is based on the byte values,
# Reference: https://blog.zhimingwang.org/macos-lc_collate-hunt
LC_ALL=C
export LC_ALL

include build/makelib/common.mk
include build/makelib/helm.mk

.PHONY: all
all: build
.DEFAULT_GOAL := all

# ====================================================================================
# Build Options

# Controller-gen version
# f284e2e8... is master ahead of v0.5.0 which has ability to generate embedded objectmeta in CRDs
CONTROLLER_GEN_VERSION=v0.16.1

# Set GOBIN
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# set the shell to bash in case some environments use sh
SHELL := /usr/bin/env bash

# Can be used or additional go build flags
BUILDFLAGS ?=
LDFLAGS ?=
TAGS ?=

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v -n
BUILDFLAGS += -x
MAKEFLAGS += VERBOSE=1
else
MAKEFLAGS += --no-print-directory
endif

# whether to generate debug information in binaries. this includes DWARF
# and symbol tables.
DEBUG ?= 0
ifeq ($(DEBUG),0)
LDFLAGS += -s -w
endif

# platforms
PLATFORMS ?= $(ALL_PLATFORMS)
# PLATFORMS_TO_BUILD_FOR controls for which platforms to build the rook binary for
PLATFORMS_TO_BUILD_FOR ?= linux_amd64 linux_arm64
SERVER_PLATFORMS := $(filter linux_%,$(PLATFORMS))
CLIENT_PLATFORMS := $(filter-out linux_%,$(PLATFORMS))

# server projects that we build on server platforms
SERVER_PACKAGES = $(GO_PROJECT)/cmd/rook

# tests packages that will be compiled into binaries
TEST_PACKAGES = $(GO_PROJECT)/tests/integration

# the root go project
GO_PROJECT=github.com/rook/rook

# inject the version number into the golang version package using the -X linker flag
LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

# CGO_ENABLED value
CGO_ENABLED_VALUE=0

# ====================================================================================
# Setup projects

# setup go projects
GO_STATIC_PACKAGES=
ifneq ($(filter $(PLATFORM),$(CLIENT_PLATFORMS) $(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(CLIENT_PACKAGES)
endif
ifneq ($(filter $(PLATFORM),$(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(SERVER_PACKAGES)
endif

GO_BUILDFLAGS=$(BUILDFLAGS)
GO_LDFLAGS=$(LDFLAGS)
GO_TAGS=$(TAGS)

GO_TEST_PACKAGES=$(TEST_PACKAGES)
GO_TEST_FLAGS=$(TESTFLAGS)
GO_TEST_SUITE=$(SUITE)
GO_TEST_FILTER=$(TESTFILTER)

include build/makelib/golang.mk

# ====================================================================================
# Targets

build.version:
	@mkdir -p $(OUTPUT_DIR)
	@echo "$(VERSION)" > $(OUTPUT_DIR)/version

build.common: export SKIP_GEN_CRD_DOCS=true
build.common: build.version helm.build mod.check crds gen-rbac
	@$(MAKE) go.init
	@$(MAKE) go.validate
	@$(MAKE) -C images/ceph list-image

do.build.platform.%:
	@$(MAKE) PLATFORM=$* go.build

do.build.parallel: $(foreach p,$(PLATFORMS_TO_BUILD_FOR), do.build.platform.$(p))

build: build.common ## Only build for linux platform
	@$(MAKE) go.build PLATFORM=linux_$(GOHOSTARCH)
	@$(MAKE) -C images PLATFORM=linux_$(GOHOSTARCH)

build.all: build.common ## Build source code for all platforms.
ifneq ($(GOHOSTARCH),amd64)
	$(error cross platform image build only supported on amd64 host currently)
endif
	@$(MAKE) do.build.parallel
	@$(MAKE) -C images build.all

install: build.common
	@$(MAKE) go.install

check test: ## Runs unit tests.
	@$(MAKE) go.test.unit

test-integration: ## Runs integration tests.
	@$(MAKE) go.test.integration

lint: ## Check syntax and styling of go sources.
	@$(MAKE) go.init
	@$(MAKE) go.lint

vet: ## Runs lint checks on go sources.
	@$(MAKE) go.init
	@$(MAKE) go.vet

fmt: ## Check formatting of go sources.
	@$(MAKE) go.init
	@$(MAKE) go.fmt

codegen: ${CODE_GENERATOR} ## Run code generators.
	@build/codegen/codegen.sh

mod.check: go.mod.check ## Check if any go modules changed.
mod.update: go.mod.update ## Update all go modules.

clean: ## Remove all files that are created by building.
	@$(MAKE) go.mod.clean
	@$(MAKE) -C images clean
	@rm -fr $(OUTPUT_DIR) $(WORK_DIR)

distclean: clean ## Remove all files that are created by building or configuring.
	@rm -fr $(CACHE_DIR)

prune: ## Prune cached artifacts.
	@$(MAKE) -C images prune

docs: helm-docs

crds: $(CONTROLLER_GEN) $(YQ)
	@echo Updating CRD manifests
	@build/crds/build-crds.sh $(CONTROLLER_GEN) $(YQ)
	@GOBIN=$(GOBIN) build/crds/generate-crd-docs.sh

gen-rbac: $(HELM) $(YQ) ## Generate RBAC from Helm charts
	@# output only stdout to the file; stderr for debugging should keep going to stderr
	HELM=$(HELM) ./build/rbac/gen-common.sh
	HELM=$(HELM) ./build/rbac/gen-nfs-rbac.sh
	HELM=$(HELM) ./build/rbac/gen-psp.sh

helm-docs: $(HELM_DOCS) ## Use helm-docs to generate documentation from helm charts
	$(HELM_DOCS) -c deploy/charts/rook-ceph \
		-o ../../../Documentation/Helm-Charts/operator-chart.md \
		-t ../../../Documentation/Helm-Charts/operator-chart.gotmpl.md \
		-t ../../../Documentation/Helm-Charts/_templates.gotmpl
	$(HELM_DOCS) -c deploy/charts/rook-ceph-cluster \
		-o ../../../Documentation/Helm-Charts/ceph-cluster-chart.md \
		-t ../../../Documentation/Helm-Charts/ceph-cluster-chart.gotmpl.md \
		-t ../../../Documentation/Helm-Charts/_templates.gotmpl

check-helm-docs:
	@$(MAKE) helm-docs
	@git diff --exit-code || { \
	echo "Please run 'make helm-docs' locally, commit the updated docs, and push the change. See https://rook.io/docs/rook/latest/Contributing/documentation/#making-docs" ; \
	exit 2 ; \
	};
check.docs:
	@$(MAKE) docs
	@tests/scripts/validate_modified_files.sh docs


docs-preview: ## Preview the documentation through mkdocs
	mkdocs serve

docs-build:  ## Build the documentation to the `site/` directory
	mkdocs build --strict

generate-docs-crds: ## Build the documentation for CRD
	@GOBIN=$(GOBIN) build/crds/generate-crd-docs.sh

.PHONY: all build.common
.PHONY: build build.all install test check vet fmt codegen mod.check clean distclean prune

# ====================================================================================
# Help
define HELPTEXT
Options:
    DEBUG        Whether to generate debug symbols. Default is 0.
    IMAGES       Backend images to make. All by default. See: /rook/images/ dir
    PLATFORM     The platform to build.
    SUITE        The test suite to run.
    TESTFILTER   Tests to run in a suite.
    VERSION      The version information compiled into binaries.
                 The default is obtained from git.
    V            Set to 1 enable verbose build. Default is 0.
endef
export HELPTEXT
.PHONY: help
help: ## Show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@grep --no-filename -E '^[a-zA-Z_%-. ]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "$$HELPTEXT"
