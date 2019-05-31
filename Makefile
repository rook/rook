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
LONGHAUL_TEST_PACKAGES = $(GO_PROJECT)/tests/longhaul

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
GO_LONGHAUL_TEST_PACKAGES=$(LONGHAUL_TEST_PACKAGES)
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

build.common: build.version helm.build
	@$(MAKE) go.init
	@$(MAKE) go.validate

do.build.platform.%:
	@$(MAKE) PLATFORM=$* go.build

do.build.parallel: $(foreach p,$(PLATFORMS), do.build.platform.$(p))

build: build.common
	@$(MAKE) go.build
# if building on non-linux platforms, also build the linux container
ifneq ($(GOOS),linux)
	@$(MAKE) go.build PLATFORM=linux_$(GOHOSTARCH)
endif
	@$(MAKE) -C images PLATFORM=linux_$(GOHOSTARCH)

build.all: build.common
ifneq ($(GOHOSTARCH),amd64)
	$(error cross platform image build only supported on amd64 host currently)
endif
	@$(MAKE) do.build.parallel
	@$(MAKE) -C images build.all

install: build.common
	@$(MAKE) go.install

check test:
	@$(MAKE) go.test.unit

test-integration:
	@$(MAKE) go.test.integration

lint:
	@$(MAKE) go.init
	@$(MAKE) go.lint

vet:
	@$(MAKE) go.init
	@$(MAKE) go.vet

fmt:
	@$(MAKE) go.init
	@$(MAKE) go.fmt

codegen:
	@build/codegen/codegen.sh

vendor: go.vendor
vendor.check: go.vendor.check
vendor.update: go.vendor.update

clean:
	@$(MAKE) -C images clean
	@rm -fr $(OUTPUT_DIR) $(WORK_DIR)

distclean: go.distclean clean
	@rm -fr $(CACHE_DIR)

prune:
	@$(MAKE) -C images prune

csv-ceph:
	@cluster/olm/ceph/generate-rook-csv.sh $(CSV_VERSION) $(CSV_PLATFORM) $(ROOK_OP_VERSION)

.PHONY: all build.common cross.build.parallel
.PHONY: build build.all install test check vet fmt codegen vendor clean distclean prune

# ====================================================================================
# Help

define HELPTEXT
Usage: make <OPTIONS> ... <TARGETS>

Targets:
    build              Build source code for host platform.
    build.all          Build source code for all platforms.
                       Best done in the cross build container
                       due to cross compiler dependencies.
    check              Runs unit tests.
    clean              Remove all files that are created by building.
    codegen            Run code generators.
    csv-ceph           Generate a CSV file for OLM.
    distclean          Remove all files that are created
                       by building or configuring.
    fmt                Check formatting of go sources.
    lint               Check syntax and styling of go sources.
    help               Show this help info.
    prune              Prune cached artifacts.
    test               Runs unit tests.
    test-integration   Runs integration tests.
    vendor             Update vendor dependencies.
    vendor.check       Checks if vendor dependencies changed.
    vendor.update      Update all vendor dependencies.
    vet                Runs lint checks on go sources.

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
help:
	@echo "$$HELPTEXT"
