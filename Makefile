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

# set the shell to bash in case some environments use sh
.PHONY: all
all: build

include build/makelib/common.mk

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

# the working directory to store packages and intermediate build files
ifeq ($(origin WORKDIR), undefined)
WORKDIR := $(abspath .work)
endif
ifeq ($(origin DOWNLOADDIR), undefined)
DOWNLOADDIR := $(abspath .download)
endif

# bin and relase dirs
ifeq ($(origin BIN_DIR),undefined)
BIN_DIR := $(abspath bin)
endif
ifeq ($(origin RELEASE_DIR), undefined)
RELEASE_DIR := $(abspath release)
endif

# platforms where we only build client bits
CLIENT_PLATFORMS ?= darwin_amd64 windows_amd64

# platforms for which we client and server bits.
SERVER_PLATFORMS ?= linux_arm linux_amd64 linux_arm64

# the platforms to build
PLATFORMS ?= $(SERVER_PLATFORMS) $(CLIENT_PLATFORMS)

# client projects that we build on all platforms
CLIENT_PACKAGES = $(GO_PROJECT)

# server projects that we build on server platforms
SERVER_PACKAGES = $(GO_PROJECT)/cmd/rookd

# the root go project
GO_PROJECT=github.com/rook/rook

# set the version number. you should not need to do this
# for the majority of scenarios.
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --dirty --always --tags)
endif
LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

# the release channel. Can be set to master, alpha, beta or stable
CHANNEL ?=

# ====================================================================================
# Setup Go projects

GO_STATIC_PACKAGES=
ifneq ($(filter $(GOOS)_$(GOARCH),$(CLIENT_PLATFORMS) $(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(CLIENT_PACKAGES)
endif
ifneq ($(filter $(GOOS)_$(GOARCH),$(SERVER_PLATFORMS)),)
GO_STATIC_PACKAGES += $(SERVER_PACKAGES)
endif

GO_BUILDFLAGS=$(BUILDFLAGS)
GO_LDFLAGS=$(LDFLAGS)
GO_TAGS=$(TAGS)

GO_BIN_DIR = $(BIN_DIR)
GO_WORK_DIR ?= $(WORKDIR)
GO_TOOLS_DIR ?= $(DOWNLOADDIR)/tools
GO_PKG_DIR ?= $(WORKDIR)/pkg

include build/makelib/golang.mk

# ====================================================================================
# Setup Distribution

RELEASE_VERSION=$(VERSION)
RELEASE_CHANNEL=$(CHANNEL)
RELEASE_BIN_DIR=$(BIN_DIR)
RELEASE_PLATFORMS=$(PLATFORMS)
include build/makelib/release.mk

# ====================================================================================
# Targets

build.common:
	@$(MAKE) go.init
	@$(MAKE) go.validate

do.build:
	@$(MAKE) go.build
	@$(MAKE) -C images

do.build.platform.%:
	@$(MAKE) GOOS=$(word 1, $(subst _, ,$*)) GOARCH=$(word 2, $(subst _, ,$*)) do.build

do.build.parallel: $(foreach p,$(PLATFORMS), do.build.platform.$(p))

build: build.common
	@$(MAKE) go.build
# if building on the mac, also build the linux container
ifneq ($(GOOS),linux)
	@$(MAKE) go.build GOOS=linux GOARCH=amd64
	@$(MAKE) -C images GOOS=linux GOARCH=amd64
else
	@$(MAKE) -C images
endif

build.all: build.common
	@$(MAKE) do.build.parallel

install: build.common
	@$(MAKE) go.install

check test: build.common
	@$(MAKE) go.test

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

clean: go.clean
	@rm -fr $(WORKDIR) $(RELEASE_DIR) $(BIN_DIR)
	@$(MAKE) release.cleanup
	@$(MAKE) -C images clean

distclean: go.distclean clean
	@rm -fr $(DOWNLOADDIR)

release: build.all
	@$(MAKE) -C images build.all
	@$(MAKE) release.build

publish:
ifneq ($(filter master alpha beta stable, $(CHANNEL)),)
	@$(MAKE) release.publish
else
	@echo skipping publish. invalid channel "$(CHANNEL)"
endif

promote:
ifneq ($(filter master alpha beta stable, $(CHANNEL)),)
	@$(MAKE) release.promote
else
	@echo skipping promote. invalid channel "$(CHANNEL)"
endif

prune:
	@$(MAKE) -C images prune

.PHONY: build.common cross.build.parallel
.PHONY: build build.all install test check vet fmt vendor clean distclean release publish promote prune

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build       Build source code for host platform.'
	@echo '    build.all   Build source code for all platforms.'
	@echo '                Best done in the cross build container'
	@echo '                due to cross compiler dependencies.'
	@echo '    check       Runs unit tests.'
	@echo '    clean       Remove all files that are created '
	@echo '                by building.'
	@echo '    distclean   Remove all files that are created '
	@echo '                by building or configuring.'
	@echo '    fmt         Check formatting of go sources.'
	@echo '    lint        Check syntax and styling of go sources.'
	@echo '    help        Show this help info.'
	@echo '    prune       Prune cached artifacts.'
	@echo '    vendor      Installs vendor dependencies.'
	@echo '    vet         Runs lint checks on go sources.'
	@echo ''
	@echo 'Release Targets:'
	@echo '    release     Builds all release artifacts including'
	@echo '                container images for all platforms.'
	@echo '    publish     Publishes all release artifacts to'
	@echo '                appropriate public repositories.'
	@echo '    promote     Promotes a published release to a'
	@echo '                release channel.'
	@echo ''
	@echo 'Options:'
	@echo '    DEBUG        Whether to generate debug symbols. Default is 0.'
	@echo '    GOARCH       The arch to build.'
	@echo '    GOOS         The OS to build for.'
	@echo '    VERSION      The version information compiled into binaries.'
	@echo '                 The default is obtained from git.'
	@echo '    V            Set to 1 enable verbose build. Default is 0.'
	@echo ''
	@echo 'Advanced Options:'
	@echo '    CHANNEL      Sets the release channel. Can be set to master,'
	@echo '                 alpha, beta, or stable. Default is not set.'
	@echo '    DOWNLOADDIR  A directory where downloaded files and other'
	@echo '                 files used during the build are cached. These'
	@echo '                 files help speedup the build by avoiding network'
	@echo '                 transfers. Its safe to use these files across builds.'
	@echo '    WORKDIR      The working directory where most build files'
	@echo '                 are stored. The default is .work'
