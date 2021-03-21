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

include build/makelib/common.mk

.PHONY: all
all: build

# ====================================================================================
# Build Options

# Controller-gen version
CONTROLLER_GEN_VERSION=v0.4.1

# Set GOBIN
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# set the shell to bash in case some environments use sh
SHELL := /bin/bash

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
SERVER_PLATFORMS := $(filter linux_%,$(PLATFORMS))
CLIENT_PLATFORMS := $(filter-out linux_%,$(PLATFORMS))

# server projects that we build on server platforms
SERVER_PACKAGES = $(GO_PROJECT)/cmd/rook $(GO_PROJECT)/cmd/rookflex

# tests packages that will be compiled into binaries
TEST_PACKAGES = $(GO_PROJECT)/tests/integration

# the root go project
GO_PROJECT=github.com/rook/rook

# inject the version number into the golang version package using the -X linker flag
LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

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

# setup helm charts
include build/makelib/helm.mk

# ====================================================================================
# Targets

build.version:
	@mkdir -p $(OUTPUT_DIR)
	@echo "$(VERSION)" > $(OUTPUT_DIR)/version

build.common: build.version helm.build mod.check
	@$(MAKE) go.init
	@$(MAKE) go.validate

do.build.platform.%:
	@$(MAKE) PLATFORM=$* go.build

do.build.parallel: $(foreach p,$(PLATFORMS), do.build.platform.$(p))

build: build.common ## Build source code for host platform.
	@$(MAKE) go.build
# if building on non-linux platforms, also build the linux container
ifneq ($(GOOS),linux)
	@$(MAKE) go.build PLATFORM=linux_$(GOHOSTARCH)
endif
	@$(MAKE) -C images PLATFORM=linux_$(GOHOSTARCH)

build.all: build.common ## Build source code for all platforms. Best done in the cross build container due to cross compiler dependencies.
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

codegen: ## Run code generators.
	@build/codegen/codegen.sh

mod.check: go.mod.check ## Check if any go modules changed.
mod.update: go.mod.update ## Update all go modules.

clean: csv-clean ## Remove all files that are created by building.
	@$(MAKE) go.mod.clean
	@$(MAKE) -C images clean
	@rm -fr $(OUTPUT_DIR) $(WORK_DIR)

distclean: clean ## Remove all files that are created by building or configuring.
	@rm -fr $(CACHE_DIR)

prune: ## Prune cached artifacts.
	@$(MAKE) -C images prune

csv-ceph: crds-gen ## Generate a CSV file for OLM.
	@echo Generating CSV manifests
	@cluster/olm/ceph/generate-rook-csv.sh $(CSV_VERSION) $(CSV_PLATFORM) $(ROOK_OP_VERSION)

csv-clean: ## Remove existing OLM files.
	@rm -fr cluster/olm/ceph/deploy/* cluster/olm/ceph/templates/*

controller-gen:
ifeq (, $(shell command -v controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION);\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell command -v controller-gen)
endif

crds-gen: controller-gen
	@echo Updating CRD manifests
	@build/crds/build-crds.sh $(CONTROLLER_GEN)

.PHONY: all build.common cross.build.parallel
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
