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
CONTROLLER_GEN_VERSION=v0.19.0
CT_VERSION := v3.13.0
KUSTOMIZE_VERSION := v5.3.0
MARKDOWNLINT_IMAGE_VERSION :=v0.21.0
SHELLCHECK_VERSION := v0.10.0

# Configuration for the yamllint contaioner image:
#
# version/tag settings(can be overridden from the cmdline):
# Uncomment/Set only what you need.
# # Priority: SHA > TAG > VERSION (defaulting to :latest)
#
# sha:
YAMLLINT_IMAGE_SHA:=
# this is the sha of the "latest" tag as of 2026-03-12:
#YAMLLINT_IMAGE_SHA := sha256:3e9eb827ab2b12a5ea5f49d4257bb3aca94bba9f1ba427c8bc7f2456385a5204
# tag:
YAMLLINT_IMAGE_TAG :=
# working tag:
#YAMLLINT_IMAGE_TAG := 1.26
# version:
YAMLLINT_VERSION :=
# many specific versions don't work as tags!
#YAMLLINT_VERSION := 1.35.1

# include here and not earlier so that the version numbers are available
# where needed
include build/makelib/common.mk
include build/makelib/helm.mk


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
# Required for go-ceph rgw/admin account APIs which are gated behind
# the ceph_preview build tag. Can be removed when go-ceph promotes
# the account API out of preview.
TAGS ?= ceph_preview

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

CHECKMAKE=go run github.com/checkmake/checkmake/cmd/checkmake@v0.3.2

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

# This target exists only for setting the variable
.PHONY: build.common.var
build.common.var: export SKIP_GEN_CRD_DOCS=true

.PHONY: build.common
build.common:  build.common.var build.version helm.build mod.check crds gen-rbac
	@$(MAKE) go.init
	@$(MAKE) go.validate
	@$(MAKE) -C images/ceph list-image

do.build.platform.%:
	@$(MAKE) PLATFORM=$* go.build

.PHONY: do.build.parallel
do.build.parallel: $(foreach p,$(PLATFORMS_TO_BUILD_FOR), do.build.platform.$(p))

.PHONY: build
build: build.common ## Only build for linux platform
	@$(MAKE) go.build PLATFORM=linux_$(GOARCH)
	@$(MAKE) -C images PLATFORM=linux_$(GOARCH)

.PHONY: build.all
build.all: build.common ## Build source code for all platforms.
ifneq ($(GOHOSTARCH),amd64)
	$(error cross platform image build only supported on amd64 host currently)
endif
	@$(MAKE) do.build.parallel
	@$(MAKE) -C images build.all

.PHONY: install
install: build.common
	@$(MAKE) go.install

.PHONY: check
check: lint.fast test build ## Runs checks (some linters, build, and unit tests)

.PHONY: test
test: ## Runs unit tests.
	@$(MAKE) go.test.unit

test-integration: ## Runs integration tests.
	@$(MAKE) go.test.integration

.PHONY: vet
vet: ## Runs lint checks on go sources.
	@$(MAKE) go.init
	@$(MAKE) go.vet

.PHONY: fmt
fmt: $(YQ) ## Check formatting of go sources.
	@$(MAKE) go.fmt

fmt-fix: $(YQ) ## Reformatting of go sources.
	@$(MAKE) go.fmt-fix

golangci-lint: $(YQ)
	@$(MAKE) go.golangci-lint

.PHONY: go.lint
go.lint: vet fmt golangci-lint ## run various go linting tools

.PHONY: markdownlint
markdownlint: ## Check formatting of documentation sources
	@$(MARKDOWNLINT) "Documentation/**/*.md" "#Documentation/Helm-Charts/**" --config .markdownlint-cli2.cjs

.PHONY: markdownlint.fix
markdownlint.fix: ## Check and fix formatting of documentation sources
	@$(MARKDOWNLINT) "Documentation/**/*.md" "#Documentation/Helm-Charts/**" --fix --config .markdownlint-cli2.cjs

.PHONY: yamllint
yamllint:
	$(YAMLLINT) -c .yamllint deploy/examples/

.PHONY: helm.lint
helm.lint: | $(HELM) $(KUSTOMIZE) ## Check the helm charts
	$(CT) lint --charts=./deploy/charts/rook-ceph,./deploy/charts/rook-ceph-cluster --validate-yaml=false --validate-maintainers=false --validate-chart-schema=false
	$(HELM) -n rook-ceph template deploy/charts/rook-ceph > templated.yaml
	$(HELM)  -n rook-ceph template deploy/charts/rook-ceph-cluster >> templated.yaml
	echo 'resources: [templated.yaml]' > kustomization.yaml
	$(KUSTOMIZE) build >/dev/null
	rm templated.yaml kustomization.yaml

.PHONY: lint
lint: lint.fast pylint ## Run various linters

.PHONY: lint.fast
lint.fast: yamllint checkmake shellcheck markdownlint go.lint helm.lint ## Run a few rather lightweight/fast linters

.PHONY: pylint
pylint:
	pylint $(shell find $(ROOT_DIR) -name '*.py') -E

.PHONY: checkmake
checkmake:
	@$(CHECKMAKE) Makefile

.PHONY: shellcheck
shellcheck: | $(SHELLCHECK)
	$(SHELLCHECK) --severity=warning --format=gcc --shell=bash $(shell find $(ROOT_DIR) -type f -name '*.sh') build/reset build/sed-in-place

.PHONY: gen.codegen
gen.codegen: codegen
.PHONY: codegen
codegen: ${CODE_GENERATOR} ## Run code generators.
	@build/codegen/codegen.sh

.PHONY: mod.check
mod.check: go.mod.check ## Check if any go modules changed.

.PHONY: mod.update
mod.update: go.mod.update ## Update all go modules.

.PHONY: clean
clean: ## Remove all files that are created by building.
	@$(MAKE) helm.dependency.clean
	@$(MAKE) go.mod.clean
	@$(MAKE) -C images clean
	@rm -fr $(OUTPUT_DIR) $(WORK_DIR)

.PHONY: distclean
distclean: clean ## Remove all files that are created by building or configuring.
	@rm -fr $(CACHE_DIR)

.PHONY: prune
prune: ## Prune cached artifacts.
	@$(MAKE) -C images prune

.PHONY: gen.crds
gen.crds: crds
.PHONY: crds
crds: $(CONTROLLER_GEN) $(YQ)
	@echo Updating CRD manifests
	@build/crds/build-crds.sh $(CONTROLLER_GEN) $(YQ)
	@GOBIN=$(GOBIN) build/crds/generate-crd-docs.sh

.PHONY: gen.rbac
gen.rbac: gen-rbac
.PHONY: gen-rbac
gen-rbac: $(HELM) $(YQ) helm.dependency.build ## Generate RBAC from Helm charts
	@# output only stdout to the file; stderr for debugging should keep going to stderr
	HELM=$(HELM) ./build/rbac/gen-common.sh
	HELM=$(HELM) ./build/rbac/gen-nfs-rbac.sh

.PHONY: gen.docs
gen.docs: docs ## generate docs
.PHONY: docs
docs: helm-docs ## generate documentation
.PHONY: gen.helm-docs
gen.helm-docs: helm-docs
helm-docs: $(HELM_DOCS) ## Use helm-docs to generate documentation from helm charts
	$(HELM_DOCS) -c deploy/charts/rook-ceph \
		-o ../../../Documentation/Helm-Charts/operator-chart.md \
		-t ../../../Documentation/Helm-Charts/operator-chart.gotmpl.md \
		-t ../../../Documentation/Helm-Charts/_templates.gotmpl
	$(HELM_DOCS) -c deploy/charts/rook-ceph-cluster \
		-o ../../../Documentation/Helm-Charts/ceph-cluster-chart.md \
		-t ../../../Documentation/Helm-Charts/ceph-cluster-chart.gotmpl.md \
		-t ../../../Documentation/Helm-Charts/_templates.gotmpl

docs-preview: ## Preview the documentation through mkdocs
	mkdocs serve

docs-build:  ## Build the documentation to the `site/` directory
	mkdocs build --strict

.PHONY: gen.crd-docs
gen.crd-docs: generate-docs-crds
generate-docs-crds: ## Build the documentation for CRD
	@GOBIN=$(GOBIN) build/crds/generate-crd-docs.sh

.PHONY: generate
generate: gen.codegen gen.crds gen.rbac gen.docs gen.crd-docs ## Update all generated files (code, manifests, charts, and docs).



# ====================================================================================
# Help
# available options:
define HELPTEXT
    DEBUG        Whether to generate debug symbols. Default is 0.
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
	@echo "available options:"
	@echo "$$HELPTEXT"
