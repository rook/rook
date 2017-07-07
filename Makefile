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

# client projects that we build on all platforms
CLIENT_PACKAGES = $(GO_PROJECT)/cmd/rookctl

# server projects that we build on server platforms
SERVER_PACKAGES = $(GO_PROJECT)/cmd/rook

# the root go project
GO_PROJECT=github.com/rook/rook

LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

# ====================================================================================
# Setup Go projects

GO_STATIC_PACKAGES=
ifneq ($(filter $(PLATFORM),$(CLIENT_PLATFORMS) $(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(CLIENT_PACKAGES)
endif
ifneq ($(filter $(PLATFORM),$(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(SERVER_PACKAGES)
endif

GO_INTEGRATION_TESTS_SUBDIRS = tests/smoke

GO_BUILDFLAGS=$(BUILDFLAGS)
GO_LDFLAGS=$(LDFLAGS)
GO_TAGS=$(TAGS)
GO_TEST_FLAGS=$(TESTFLAGS)
GO_TEST_SUITE=$(SUITE)

include build/makelib/golang.mk

# ====================================================================================
# Targets

build.common:
	@$(MAKE) go.init
	@$(MAKE) go.validate

do.build.platform.%:
	@$(MAKE) GOOS=$(word 1, $(subst _, ,$*)) GOARCH=$(word 2, $(subst _, ,$*)) go.build

do.build.parallel: $(foreach p,$(PLATFORMS), do.build.platform.$(p))

build: build.common
	@$(MAKE) go.build
# if building on non-linux platforms, also build the linux container
ifneq ($(GOOS),linux)
	@$(MAKE) go.build GOOS=linux GOARCH=amd64
endif
	@$(MAKE) -C images

build.all: build.common
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

vendor: go.vendor

clean:
	@$(MAKE) -C images clean
	@rm -fr $(OUTPUT_DIR) $(WORK_DIR)

distclean: go.distclean clean
	@rm -fr $(CACHE_DIR)

prune:
	@$(MAKE) -C images prune

.PHONY: all build.common cross.build.parallel
.PHONY: build build.all install test check vet fmt vendor clean distclean prune

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build              Build source code for host platform.'
	@echo '    build.all          Build source code for all platforms.'
	@echo '                       Best done in the cross build container'
	@echo '                       due to cross compiler dependencies.'
	@echo '    check              Runs unit tests.'
	@echo '    clean              Remove all files that are created '
	@echo '                       by building.'
	@echo '    distclean          Remove all files that are created '
	@echo '                       by building or configuring.'
	@echo '    fmt                Check formatting of go sources.'
	@echo '    lint               Check syntax and styling of go sources.'
	@echo '    help               Show this help info.'
	@echo '    prune              Prune cached artifacts.'
	@echo '    test               Runs unit tests.'
	@echo '    test-integration   Runs integration tests.'
	@echo '    vendor             Installs vendor dependencies.'
	@echo '    vet                Runs lint checks on go sources.'
	@echo ''
	@echo 'Options:'
	@echo '    DEBUG        Whether to generate debug symbols. Default is 0.'
	@echo '    PLATFORM     The platform to build.'
	@echo '    SUITE        The test suite to run.'
	@echo '    VERSION      The version information compiled into binaries.'
	@echo '                 The default is obtained from git.'
	@echo '    V            Set to 1 enable verbose build. Default is 0.'
