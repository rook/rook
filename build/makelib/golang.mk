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

# These target do not use cgo ir SWIG and contain all pure go code. They will be
# statically or dynamically linked depending on whether they import the standard
# net, os/user packages.
GO_NONSTATIC_PACKAGES ?=

# These targets do not use cgo ir SWIG and contain all pure go code. They will
# be forced to link statically even if they import the net or os/user packages.
GO_STATIC_PACKAGES ?=

# These targets are a mix of go and non-go code. They will be linked statically
# or dynamically based on the linker flags passed through ldflags.
GO_CGO_PACKAGES ?=

ifeq ($(GO_NONSTATIC_PACKAGES)$(GO_STATIC_PACKAGES)$(GO_CGO_PACKAGES),)
$(error please set GO_STATIC_PACKAGES, GO_NONSTATIC_PACKAGES, and/or GO_CGO_PACKAGES prior to including golang.mk)
endif

# Optional. These are sudirs that we look for all go files to test, vet, and fmt
GO_SUBDIRS ?= cmd pkg

# Optional directories (relative to CURDIR)
GO_BIN_DIR ?= bin
GO_TOOLS_DIR ?= tools
GO_VENDOR_DIR ?= vendor
GO_PKG_DIR ?=

# Optional build flags passed to go tools
GO_TOOL_FLAGS ?=

# Optional CGO flags directories
CGO_CFLAGS ?=
CGO_CXXFLAGS ?=
CGO_LDFLAGS ?=

# Optional prerequisities for CGO builds
CGO_PREREQS ?=

# Optional OS and ARCH to build
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# ====================================================================================
# Setup go environment

GO_SUPPORTED_VERSIONS ?= 1.6|1.7

GOROOT = $(shell go env GOROOT)
GOPATH = $(shell go env GOPATH)
GOHOSTOS = $(shell go env GOHOSTOS)
GOHOSTARCH = $(shell go env GOHOSTARCH)

GO_ALL_PACKAGES := $(foreach t,$(GO_SUBDIRS),$(GO_PROJECT)/$(t)/...)

unexport CGO_ENABLED
export CGO_CFLAGS CGO_CPPFLAGS CGO_LDFLAGS GLIDE_HOME

# setup glide
GLIDE_HOME := $(abspath .glide)
GLIDE := $(GO_TOOLS_DIR)/glide

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go

GO_OUT_DIR := $(abspath $(GO_BIN_DIR)/$(GOOS)_$(GOARCH))

ifeq ($(GOOS),windows)
GO_OUT_EXT := ".exe"
endif

ifneq ($(GO_PKG_DIR),)
GO_PKG_FLAGS := -pkgdir $(abspath $(GO_PKG_DIR)/$(GOOS)_$(GOARCH))
GO_PKG_STATIC_FLAGS := -pkgdir $(abspath $(GO_PKG_DIR)/$(GOOS)_$(GOARCH)_static) -installsuffix static
else
GO_PKG_STATIC_FLAGS := -installsuffix static
endif

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

.PHONY: go.init
go.init: $(GO_VENDOR_DIR)/vendor.stamp

-include go.init
endif

.PHONY: go.build
go.build: go.vet go.fmt $(CGO_PREREQS)
	$(foreach p,$(GO_NONSTATIC_PACKAGES),@CGO_ENABLED=1 $(GO) build -v -i $(GO_PKG_FLAGS) -o $(GO_OUT_DIR)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_TOOL_FLAGS) $(p))
	$(foreach p,$(GO_CGO_PACKAGES),@CGO_ENABLED=1 $(GO) build -v -i $(GO_PKG_FLAGS) -o $(GO_OUT_DIR)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_TOOL_FLAGS) $(p))
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=0 $(GO) build -v -i $(GO_PKG_STATIC_FLAGS) -o $(GO_OUT_DIR)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_TOOL_FLAGS) $(p))

.PHONY: go.install
go.install: go.vet go.fmt $(CGO_PREREQS)
	$(foreach p,$(GO_NONSTATIC_PACKAGES),@CGO_ENABLED=1 $(GO) install -v $(GO_PKG_FLAGS) $(GO_TOOL_FLAGS) $(p))
	$(foreach p,$(GO_CGO_PACKAGES),@CGO_ENABLED=1 $(GO) install -v $(GO_PKG_FLAGS) $(GO_TOOL_FLAGS) $(p))
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=0 $(GO) install -v $(GO_PKG_STATIC_FLAGS) $(GO_TOOL_FLAGS) $(p))

.PHONY: go.test
go.test: go.vet go.fmt $(CGO_PREREQS)
#   this is disabled since it looks like there's a bug in go test where it attempts to install cmd/cgo
#	@$(GOHOST) test -v -i $(GO_PKG_FLAGS) $(GO_TOOL_FLAGS) $(GO_ALL_PACKAGES)
	@$(GOHOST) test -cover $(GO_PKG_FLAGS) $(GO_TOOL_FLAGS) $(GO_ALL_PACKAGES)

.PHONY: go.vet
go.vet:
	@$(GOHOST) vet $(GO_TOOL_FLAGS) $(GO_ALL_PACKAGES)

.PHONY: go.fmt
go.fmt:
	@$(GOHOST) fmt $(GO_ALL_PACKAGES)

.PHONY: go.vendor
go.vendor $(GO_VENDOR_DIR)/vendor.stamp: $(GO_TOOLS_DIR)/glide
	@mkdir -p $(GLIDE_HOME)
	@$(GLIDE) install
	@touch $(GO_VENDOR_DIR)/vendor.stamp

$(GO_TOOLS_DIR)/glide:
	@echo "installing glide"
	@mkdir -p $(GO_TOOLS_DIR)
	@curl -sL https://github.com/Masterminds/glide/releases/download/v0.12.3/glide-v0.12.3-$(GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(GO_TOOLS_DIR)
	@mv $(GO_TOOLS_DIR)/$(GOHOSTOS)-$(GOHOSTARCH)/glide $(GO_TOOLS_DIR)/glide
	@rm -r $(GO_TOOLS_DIR)/$(GOHOSTOS)-$(GOHOSTARCH)

.PHONY: go.clean
go.clean: ;
	@rm -rf $(GO_BIN_DIR)/*
ifneq ($(GO_PKG_DIR),)
	@rm -rf $(GO_PKG_DIR)
endif

.PHONY: go.distclean
go.distclean: go.clean
	@rm -rf  $(GO_TOOLS_DIR) $(GO_VENDOR_DIR) $(GLIDE_HOME)
