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

GO_SUPPORTED_VERSIONS ?= 1.7|1.8

GO_PACKAGES := $(foreach t,$(GO_SUBDIRS),$(GO_PROJECT)/$(t)/...)
GO_INTEGRATION_TEST_PACKAGES := $(foreach t,$(GO_INTEGRATION_TESTS_SUBDIRS),$(GO_PROJECT)/$(t)/...)

ifneq ($(GO_TEST_SUITE),)
GO_TEST_FLAGS += -run '$(GO_TEST_SUITE)'
endif

GOPATH := $(shell go env GOPATH)

# setup tools used during the build
GLIDE_VERSION=v0.12.3
GLIDE_HOME := $(abspath $(CACHE_DIR)/glide)
GLIDE := $(TOOLS_HOST_DIR)/glide-$(GLIDE_VERSION)
GLIDE_YAML := $(ROOT_DIR)/glide.yaml
GLIDE_LOCK := $(ROOT_DIR)/glide.lock
GLIDE_INSTALL_STAMP := $(GO_VENDOR_DIR)/vendor.stamp
GOLINT := $(TOOLS_HOST_DIR)/golint
GOJUNIT := $(TOOLS_HOST_DIR)/go-junit-report
export GLIDE_HOME

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go

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

GO_STATIC_FLAGS = $(GO_BUILDFLAGS) $(GO_PKG_STATIC_FLAGS) -installsuffix static -tags '$(GO_TAGS)' -ldflags '$(GO_LDFLAGS)'

# ====================================================================================
# Targets

ifeq ($(filter help clean distclean prune go.clean, $(MAKECMDGOALS)),)
.PHONY: go.check
go.check:
ifneq ($(shell $(GO) version | grep -q -E '\bgo($(GO_SUPPORTED_VERSIONS))\b' && echo 0 || echo 1), 0)
	$(error unsupported go version. Please make install one of the following supported version: '$(GO_SUPPORTED_VERSIONS)')
endif
ifneq ($(realpath ../../../..), $(realpath $(GOPATH)))
	$(warning WARNING: the source directory is not relative to the GOPATH at $(GOPATH) or you are you using symlinks. The build might run into issue. Please move the source directory to be at $(GOPATH)/src/$(GO_PROJECT))
endif

-include go.check
endif

.PHONY: go.init
go.init: $(GLIDE_INSTALL_STAMP)
	@:

define go.project
go.build.packages.$(1):
	@echo === go build $(1) $(PLATFORM)
	@$(3) $(GO) build -v -i -o $(GO_OUT_DIR)/$(1)$(GO_OUT_EXT) $(4) $(2)

go.build.packages: go.build.packages.$(1)

go.install.packages.$(1):
	@echo === go install $(1) $(PLATFORM)
	@$(3) $(GO) install -v $(4) $(2)
go.install.packages: go.install.packages.$(1)
endef
$(foreach p,$(GO_STATIC_PACKAGES),$(eval $(call go.project,$(lastword $(subst /, ,$(p))),$(p),CGO_ENABLED=0,$(GO_STATIC_FLAGS))))

.PHONY: go.build
go.build:
	@$(MAKE) go.build.packages
	@$(MAKE) go.build.integration.test

.PHONY: go.install
go.install:
	@$(MAKE) go.install.packages

.PHONY: go.test.unit
go.test.unit: $(GOJUNIT)
	@echo === go test unit-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	@CGO_ENABLED=0 $(GOHOST) test -v -i -cover $(GO_STATIC_FLAGS) $(GO_PACKAGES)
	@CGO_ENABLED=0 $(GOHOST) test -v -cover $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_PACKAGES) 2>&1 | tee $(GO_TEST_OUTPUT)/unit-tests.log
	@cat $(GO_TEST_OUTPUT)/unit-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/unit-tests.xml

.PHONY: go.build.integration.test
go.build.integration.test:
	@echo === go build integration test packages $(PLATFORM)
	@CGO_ENABLED=0 $(GOHOST) test -v -i $(GO_STATIC_FLAGS) -c -o $(GO_TEST_OUTPUT)/test.integration $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY:
go.test.integration: $(GOJUNIT)
	@echo === go test integration-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	@CGO_ENABLED=0 $(GOHOST) test -v -i $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES)
	@CGO_ENABLED=0 $(GOHOST) test -v $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES) 2>&1 | tee $(GO_TEST_OUTPUT)/integration-tests.log
	@cat $(GO_TEST_OUTPUT)/integration-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/integration-tests.xml

.PHONY: go.lint
go.lint: $(GOLINT)
	@echo === go lint
	@$(GOLINT) -set_exit_status=true $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.vet
go.vet:
	@echo === go vet
	@$(GOHOST) vet $(GO_STATIC_FLAGS) $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.fmt
go.fmt:
	@gofmt_out=$$(gofmt -s -d -e $(GO_SUBDIRS) $(GO_INTEGRATION_TESTS_SUBDIRS) 2>&1) && [ -z "$${gofmt_out}" ] || (echo "$${gofmt_out}" 1>&2; exit 1)

go.validate: go.vet go.fmt

go.vendor: $(GLIDE) $(GLIDE_YAML)
	@echo === updating vendor dependencies
	@mkdir -p $(GLIDE_HOME)
	@$(GLIDE) update --strip-vendor

$(GLIDE_INSTALL_STAMP): $(GLIDE) $(GLIDE_LOCK)
	@echo === installing vendor dependencies
	@mkdir -p $(GLIDE_HOME)
	@$(GLIDE) install --strip-vendor
	@touch $@

$(GLIDE):
	@echo === installing glide
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://github.com/Masterminds/glide/releases/download/$(GLIDE_VERSION)/glide-$(GLIDE_VERSION)-$(GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/$(GOHOSTOS)-$(GOHOSTARCH)/glide $(GLIDE)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOLINT):
	@echo === installing golint
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOPATH=$(TOOLS_HOST_DIR)/tmp GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) get github.com/golang/lint/golint
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOJUNIT):
	@echo === installing go-junit-report
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOPATH=$(TOOLS_HOST_DIR)/tmp GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) get github.com/jstemmer/go-junit-report
	@rm -fr $(TOOLS_HOST_DIR)/tmp

.PHONY: go.distclean
go.distclean:
	@rm -rf $(GLIDE_INSTALL_STAMP) $(GO_VENDOR_DIR)

