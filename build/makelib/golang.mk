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

# ====================================================================================
# Makefile helper functions for golang
#

ifeq ($(GO_PROJECT),)
$(error the variable GO_PROJECT must be set prior to including golang.mk)
endif

# These targets will be statically linked.
GO_STATIC_PACKAGES ?=

ifeq ($(GO_STATIC_PACKAGES),)
$(error please set GO_STATIC_PACKAGES prior to including golang.mk)
endif

# These are the static test packages
GO_STATIC_PACKAGES ?=

# Optional. These are subdirs that we look for all go files to test, vet, and fmt
GO_SUBDIRS ?= cmd pkg

# Optional. Additional subdirs used for integration or e2e testings
GO_INTEGRATION_TESTS_SUBDIRS ?= tests

# Optional directories (relative to CURDIR)
GO_VENDOR_DIR ?= vendor
GO_PKG_DIR ?= $(WORK_DIR)/pkg

# Optional build flags passed to go tools
GO_BUILDFLAGS ?=
GO_LDFLAGS ?=
GO_TAGS ?=
GO_TEST_FLAGS ?=

# ====================================================================================
# Setup go environment

GO_SUPPORTED_VERSIONS ?= 1.11|1.12|1.13

GO_PACKAGES := $(foreach t,$(GO_SUBDIRS),$(GO_PROJECT)/$(t)/...)
GO_INTEGRATION_TEST_PACKAGES := $(foreach t,$(GO_INTEGRATION_TESTS_SUBDIRS),$(GO_PROJECT)/$(t)/integration)

ifneq ($(GO_TEST_SUITE),)
GO_TEST_FLAGS += -run '$(GO_TEST_SUITE)'
endif

ifneq ($(GO_TEST_FILTER),)
TEST_FILTER_PARAM := -testify.m '$(GO_TEST_FILTER)'
endif

GOPATH := $(shell go env GOPATH)

# setup tools used during the build
DEP_VERSION=v0.5.4
DEP := $(TOOLS_HOST_DIR)/dep-$(DEP_VERSION)
GOLINT := $(TOOLS_HOST_DIR)/golint
GOJUNIT := $(TOOLS_HOST_DIR)/go-junit-report

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go
GO_VERSION := $(shell $(GO) version | sed -ne 's/[^0-9]*\(\([0-9]\.\)\{0,4\}[0-9][^.]\).*/\1/p')

# we use a consistent version of gofmt even while running different go compilers.
# see https://github.com/golang/go/issues/26397 for more details
GOFMT_VERSION := 1.11
ifneq ($(findstring $(GOFMT_VERSION),$(GO_VERSION)),)
GOFMT := $(shell which gofmt)
else
GOFMT := $(TOOLS_HOST_DIR)/gofmt$(GOFMT_VERSION)
endif

GO_OUT_DIR := $(abspath $(OUTPUT_DIR)/bin/$(PLATFORM))
GO_TEST_OUTPUT := $(abspath $(OUTPUT_DIR)/tests/$(PLATFORM))

ifeq ($(GOOS),windows)
GO_OUT_EXT := .exe
endif

# NOTE: the install suffixes are matched with the build container to speed up the
# the build. Please keep them in sync.

# we run go build with -i which on most system's would want to install packages
# into the system's root dir. using our own pkg dir avoid thats
ifneq ($(GO_PKG_DIR),)
GO_PKG_BASE_DIR := $(abspath $(GO_PKG_DIR)/$(PLATFORM))
GO_PKG_STATIC_FLAGS := -pkgdir $(GO_PKG_BASE_DIR)_static
endif

GO_COMMON_FLAGS = $(GO_BUILDFLAGS) -tags '$(GO_TAGS)'
GO_STATIC_FLAGS = $(GO_COMMON_FLAGS) $(GO_PKG_STATIC_FLAGS) -installsuffix static  -ldflags '$(GO_LDFLAGS)'

# ====================================================================================
# Targets

ifeq ($(filter help clean distclean prune go.clean, $(MAKECMDGOALS)),)
.PHONY: go.check
go.check:
ifneq ($(shell $(GO) version | grep -q -E '\bgo($(GO_SUPPORTED_VERSIONS))\b' && echo 0 || echo 1), 0)
	$(error unsupported go version. Please make install one of the following supported version: '$(GO_SUPPORTED_VERSIONS)')
endif
ifneq ($(realpath ../../../..), $(realpath $(GOPATH)))
	$(warning WARNING: the source directory is not relative to the GOPATH at $(GOPATH) or you are using symlinks. The build might run into issue. Please move the source directory to be at $(GOPATH)/src/$(GO_PROJECT))
endif

-include go.check
endif

.PHONY: go.init
go.init: go.vendor.lite
	@:

.PHONY: go.build
go.build:
	@echo === go build $(PLATFORM)
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=0 $(GO) build -v -i -o $(GO_OUT_DIR)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_STATIC_FLAGS) $(p)${\n})
	$(foreach p,$(GO_TEST_PACKAGES) $(GO_LONGHAUL_TEST_PACKAGES),@CGO_ENABLED=0 $(GO) test -v -i -c -o $(GO_TEST_OUTPUT)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_STATIC_FLAGS) $(p)${\n})

.PHONY: go.install
go.install:
	@echo === go install $(PLATFORM)
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=0 $(GO) install -v $(GO_STATIC_FLAGS) $(p)${\n})

.PHONY: go.test.unit
go.test.unit: $(GOJUNIT)
	@echo === go test unit-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	CGO_ENABLED=0 $(GOHOST) test -v -i -cover $(GO_STATIC_FLAGS) $(GO_PACKAGES)
	CGO_ENABLED=0 $(GOHOST) test -v -cover $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_PACKAGES) 2>&1 | tee $(GO_TEST_OUTPUT)/unit-tests.log
	@cat $(GO_TEST_OUTPUT)/unit-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/unit-tests.xml

.PHONY:
go.test.integration: $(GOJUNIT)
	@echo === go test integration-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	CGO_ENABLED=0 $(GOHOST) test -v -i $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES)
	CGO_ENABLED=0 $(GOHOST) test -v -timeout 7200s $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES) $(TEST_FILTER_PARAM) 2>&1 | tee $(GO_TEST_OUTPUT)/integration-tests.log
	@cat $(GO_TEST_OUTPUT)/integration-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/integration-tests.xml

.PHONY: go.lint
go.lint: $(GOLINT)
	@echo === go lint
	@$(GOLINT) -set_exit_status=true $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.vet
go.vet:
	@echo === go vet
	@CGO_ENABLED=0 $(GOHOST) vet $(GO_COMMON_FLAGS) $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.fmt
go.fmt: $(GOFMT)
	@gofmt_out=$$($(GOFMT) -s -d -e $(GO_SUBDIRS) $(GO_INTEGRATION_TESTS_SUBDIRS) 2>&1) && [ -z "$${gofmt_out}" ] || (echo "$${gofmt_out}" 1>&2; exit 1)

go.validate: go.vet go.fmt

.PHONY: go.vendor.lite
go.vendor.lite: $(DEP)
#	dep ensure blindly updates the whole vendor tree causing everything to be rebuilt. This workaround
#	will only call dep ensure if the .lock file changes or if the vendor dir is non-existent.
	@if [ ! -d $(GO_VENDOR_DIR) ]; then \
		$(MAKE) go.vendor; \
	elif ! $(DEP) ensure -no-vendor -dry-run &> /dev/null; then \
		$(MAKE) go.vendor; \
	fi

.PHONY: go.vendor.check
go.vendor.check: $(DEP)
	@echo === checking if vendor deps changed
	@$(DEP) check -skip-vendor
	@echo === vendor deps have not changed

.PHONY: go.vendor
go.vendor: $(DEP)
	@echo === ensuring vendor dependencies are up to date
	@$(DEP) ensure

.PHONY: go.vendor.update
go.vendor.update: $(DEP)
	@echo === updating vendor dependencies
	@$(DEP) ensure -update -v

$(DEP):
	@echo === installing dep
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL -o $(DEP) https://github.com/golang/dep/releases/download/$(DEP_VERSION)/dep-$(GOHOSTOS)-$(GOHOSTARCH)
	@chmod +x $(DEP)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOLINT):
	@echo === installing golint
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOPATH=$(TOOLS_HOST_DIR)/tmp GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) get github.com/golang/lint/golint
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOFMT):
	@echo === installing gofmt$(GOFMT_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://dl.google.com/go/go$(GOFMT_VERSION).$(GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/go/bin/gofmt $(GOFMT)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOJUNIT):
	@echo === installing go-junit-report
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOPATH=$(TOOLS_HOST_DIR)/tmp GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) get github.com/jstemmer/go-junit-report
	@rm -fr $(TOOLS_HOST_DIR)/tmp

.PHONY: go.distclean
go.distclean:
	@rm -rf $(GO_VENDOR_DIR)
