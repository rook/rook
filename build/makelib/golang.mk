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
$(error the variable $$GO_PROJECT must be set prior to including golang.mk)
endif

# These targets will statically or dynamically linked depending on whether they
# import the standard net, os/user packages.
GO_PACKAGES ?=

# These targets will statically or dynamically linked depending on whether they
# import the standard net, os/user packages. Buildmode PIE will be enabled.
GO_PIE_PACKAGES ?=

# These targets will be statically linked.
GO_STATIC_PACKAGES ?=

ifeq ($(GO_PACKAGES)$(GO_PIE_PACKAGES)$(GO_STATIC_PACKAGES),)
$(error please set GO_PACKAGES, GO_PIE_PACKAGES, and/or GO_STATIC_PACKAGES prior to including golang.mk)
endif

# Optional. These are sudirs that we look for all go files to test, vet, and fmt
GO_SUBDIRS ?= cmd pkg

# Optional directories (relative to CURDIR)
GO_BIN_DIR ?= bin
GO_TOOLS_DIR ?= .tools
GO_WORK_DIR ?= .work
GO_VENDOR_DIR ?= vendor
GO_PKG_DIR ?=

# Optional build flags passed to go tools
GO_BUILDFLAGS ?=
GO_LDFLAGS ?=
GO_TAGS ?=

# ====================================================================================
# Setup go environment

include $(dir $(lastword $(MAKEFILE_LIST)))/cross.mk

GO_SUPPORTED_VERSIONS ?= 1.7|1.8

GO_ALL_PACKAGES := $(foreach t,$(GO_SUBDIRS),$(GO_PROJECT)/$(t)/...)

GOPATH := $(shell go env GOPATH)

# setup tools used during the build
GO_TOOLS_HOST_DIR := $(abspath $(GO_TOOLS_DIR)/$(GOHOSTOS)_$(GOHOSTARCH))
GLIDE_VERSION=v0.12.3
GLIDE_HOME := $(abspath $(GO_WORK_DIR)/glide)
GLIDE := $(GO_TOOLS_HOST_DIR)/glide-$(GLIDE_VERSION)
GOLINT := $(GO_TOOLS_HOST_DIR)/golint
export GLIDE_HOME

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go

GO_OUT_DIR := $(abspath $(GO_BIN_DIR)/$(GOOS)_$(GOARCH))

ifeq ($(GOOS),windows)
GO_OUT_EXT := .exe
endif

# NOTE: the install suffixes are matched with the build container to speed up the
# the build. Please keep them in sync.

# we run go build with -i which on most system's would want to install packages
# into the system's root dir. using our own pkg dir avoid thats
ifneq ($(GO_PKG_DIR),)
GO_PKG_BASE_DIR := $(abspath $(GO_PKG_DIR)/$(GOOS)_$(GOARCH))
GO_PKG_FLAGS := -pkgdir $(GO_PKG_BASE_DIR)
GO_PKG_PIE_FLAGS := -pkgdir $(GO_PKG_BASE_DIR)_pie_shared
GO_PKG_STATIC_FLAGS := -pkgdir $(GO_PKG_BASE_DIR)_static
endif

GO_FLAGS         = $(GO_BUILDFLAGS) $(GO_PKG_FLAGS) -tags '$(GO_TAGS)' -ldflags '$(GO_LDFLAGS)'
GO_PIE_FLAGS     = $(GO_BUILDFLAGS) $(GO_PKG_PIE_FLAGS) -installsuffix pie -buildmode pie -tags '$(GO_TAGS)' -ldflags '$(GO_LDFLAGS)'
GO_STATIC_FLAGS  = $(GO_BUILDFLAGS) $(GO_PKG_STATIC_FLAGS) -installsuffix static -tags '$(GO_TAGS)' -ldflags '$(GO_LDFLAGS)'

# ====================================================================================
# Targets

ifeq ($(filter help clean distclean, $(MAKECMDGOALS)),)
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
go.init: $(GO_VENDOR_DIR)/vendor.stamp
	@:

define go.project
go.build.packages.$(1):
	@echo === go build $(1) $(GOOS)_$(GOARCH)
	@CGO_ENABLED=$(3) $(GO) build -v -i -o $(GO_OUT_DIR)/$(1)$(GO_OUT_EXT) $(4) $(2)

go.build.packages: go.build.packages.$(1)

go.install.packages.$(1):
	@echo === go install $(1) $(GOOS)_$(GOARCH)
	@CGO_ENABLED=$(3) $(GO) install -v $(4) $(2)
go.install.packages: go.install.packages.$(1)
endef

$(foreach p,$(GO_PACKAGES),$(eval $(call go.project,$(lastword $(subst /, ,$(p))),$(p),1,$(GO_FLAGS))))
$(foreach p,$(GO_PIE_PACKAGES),$(eval $(call go.project,$(lastword $(subst /, ,$(p))),$(p),1,$(GO_PIE_FLAGS))))
$(foreach p,$(GO_STATIC_PACKAGES),$(eval $(call go.project,$(lastword $(subst /, ,$(p))),$(p),0,$(GO_STATIC_FLAGS))))

.PHONY: go.build
go.build:
	@$(MAKE) go.build.packages

.PHONY: go.install
go.install:
	@$(MAKE) go.install.packages

.PHONY:
go.test:
	@echo === go test
	@$(GOHOST) test -v -i -cover $(GO_FLAGS) $(GO_ALL_PACKAGES)
	@$(GOHOST) test -cover $(GO_FLAGS) $(GO_ALL_PACKAGES)

.PHONY: go.lint
go.lint: $(GOLINT)
	@echo === go lint
	@$(GOLINT) -set_exit_status=true $(GO_ALL_PACKAGES)

.PHONY: go.vet
go.vet:
	@echo === go vet
	@$(GOHOST) vet $(GO_FLAGS) $(GO_ALL_PACKAGES)

.PHONY: go.fmt
go.fmt:
	@gofmt_out=$$(gofmt -d -e $(GO_SUBDIRS) 2>&1) && [ -z "$${gofmt_out}" ] || (echo "$${gofmt_out}" 1>&2; exit 1)

go.validate: go.vet go.fmt

.PHONY: go.vendor
go.vendor $(GO_VENDOR_DIR)/vendor.stamp: $(GLIDE)
	@echo === updating vendor dependencies
	@mkdir -p $(GLIDE_HOME)
	@$(GLIDE) install --strip-vendor
	@touch $(GO_VENDOR_DIR)/vendor.stamp

$(GLIDE):
	@echo === installing glide
	@mkdir -p $(GO_TOOLS_HOST_DIR)/tmp
	@curl -sL https://github.com/Masterminds/glide/releases/download/$(GLIDE_VERSION)/glide-$(GLIDE_VERSION)-$(GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(GO_TOOLS_HOST_DIR)/tmp
	@mv $(GO_TOOLS_HOST_DIR)/tmp/$(GOHOSTOS)-$(GOHOSTARCH)/glide $(GLIDE)
	@rm -fr $(GO_TOOLS_HOST_DIR)/tmp

$(GOLINT):
	@echo === installing golint
	@mkdir -p $(GO_TOOLS_HOST_DIR)/tmp
	@GOPATH=$(GO_TOOLS_HOST_DIR)/tmp GOBIN=$(GO_TOOLS_HOST_DIR) $(GOHOST) get github.com/golang/lint/golint
	@rm -fr $(GO_TOOLS_HOST_DIR)/tmp

.PHONY: go.clean
go.clean: ;
	@rm -rf $(GO_BIN_DIR)
ifneq ($(GO_PKG_DIR),)
	@rm -rf $(GO_PKG_DIR)
endif

.PHONY: go.distclean
go.distclean: go.clean
	@rm -rf  $(GO_TOOLS_DIR) $(GO_VENDOR_DIR) $(GLIDE_HOME)
