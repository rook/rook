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

# ====================================================================================
# Build Options

# set the shell to bash in case some environments use sh
SHELL := /bin/bash

# Can be used for additional go build flags
BUILDFLAGS ?=
LDFLAGS ?=
TAGS ?=

# if set to 'dynamic' all dependencies are dynamically linked. if
# set to 'static' all dependencies will be statically linked. If set
# to 'stdlib' then the standard library will be dynamically
# linked and everything else will be statically linked.
LINKMODE ?= dynamic
ifeq ($(LINKMODE),dynamic)
TAGS += dynamic
else
ifeq ($(LINKMODE),stdlib)
TAGS += stdlib
else
TAGS += static
endif
endif

# build a position independent executable. This implies dynamic linking
# since statically-linked PIE is not supported by the linker/glibc. PIE
# is only supported on Linux.
PIE ?= 0
ifeq ($(PIE),1)
ifeq ($(LINKMODE),static)
$(error PIE only supported with dynamic linking. Set LINKMODE=dynamic or LINKMODE=stdlib.)
endif
endif

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v -n
BUILDFLAGS += -x
MAKEFLAGS += VERBOSE=1
else
MAKEFLAGS += --no-print-directory
endif

# the operating system and arch to build
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# the working directory to store packages and intermediate build files
WORKDIR ?= .work

# bin and relase dirs
BIN_DIR=bin
RELEASE_DIR=release

ALL_PLATFORMS ?= linux_amd64 linux_arm64 darwin_amd64 windows_amd64

GO_PROJECT=github.com/rook/rook

# set the version number.
ifeq ($(origin VERSION), undefined)
VERSION = $(shell git describe --dirty --always)
endif
LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

# ====================================================================================
# Setup Go projects

# support for cross compiling
include build/makelib/cross.mk

ifeq ($(GOOS),linux)

# Set the memory allocator used for ceph
CEPH_BRANCH ?= kraken

# Set the memory allocator used for ceph
ALLOCATOR ?= tcmalloc_minimal

ifeq ($(ALLOCATOR),jemalloc)
TAGS += jemalloc
else
ifeq ($(ALLOCATOR),tcmalloc_minimal)
TAGS += tcmalloc_minimal
else
endif
endif

# Set the cgo flags to link externals
CGO_CFLAGS = -I$(abspath external/build/$(CROSS_TRIPLE)/include)
CGO_LDFLAGS = -L$(abspath external/build/$(CROSS_TRIPLE)/lib)

endif

GO_BIN_DIR = $(BIN_DIR)

ifeq ($(LINKMODE),static)
GO_STATIC_PACKAGES=$(GO_PROJECT)
ifeq ($(GOOS),linux)
GO_STATIC_CGO_PACKAGES=$(GO_PROJECT)/cmd/rookd $(GO_PROJECT)/cmd/rook-operator
endif
else
GO_NONSTATIC_PACKAGES=$(GO_PROJECT)
ifeq ($(GOOS),linux)
ifeq ($(PIE),1)
GO_NONSTATIC_PIE_PACKAGES+= $(GO_PROJECT)/cmd/rookd $(GO_PROJECT)/cmd/rook-operator
else
GO_NONSTATIC_PACKAGES+= $(GO_PROJECT)/cmd/rookd $(GO_PROJECT)/cmd/rook-operator
endif
endif
endif

GO_BUILDFLAGS=$(BUILDFLAGS)
GO_LDFLAGS=$(LDFLAGS)
GO_TAGS=$(TAGS)

GO_PKG_DIR ?= $(WORKDIR)/pkg

include build/makelib/golang.mk

# ====================================================================================
# Setup Distribution

RELEASE_VERSION=$(VERSION)
RELEASE_BIN_DIR=$(BIN_DIR)
RELEASE_PLATFORMS=$(ALL_PLATFORMS)
include build/makelib/release.mk

# ====================================================================================
# External Targets

export DOWNLOADDIR

external:
ifeq ($(GOOS),linux)
	@$(MAKE) -C external CEPH_BRANCH=$(CEPH_BRANCH) ALLOCATOR=$(ALLOCATOR) PLATFORMS=$(CROSS_TRIPLE) cross
endif

external/build/$(CROSS_TRIPLE)/lib/libcephd.a:
	@$(MAKE) external

external.clean:
	@$(MAKE) -C external clean

external.distclean:
	@$(MAKE) -C external distclean

.PHONY: external external.clean external.distclean

# ====================================================================================
# Targets

dev: external/build/$(CROSS_TRIPLE)/lib/libcephd.a
	@$(MAKE) go.build
	@$(MAKE) release.build.containers.$(GOOS)_$(GOARCH)

build: external
	@$(MAKE) go.build

install: external
	@$(MAKE) go.install

check test: external
	@$(MAKE) go.test

vet: go.vet

fmt: go.fmt

vendor: go.vendor

clean: go.clean external.clean
	@rm -fr $(WORKDIR) $(RELEASE_DIR)/* $(BIN_DIR)/*

container: build
	@RELEASE_CLIENT_SERVER_PLATFORMS=$(GOOS)_$(GOARCH) build/release/release.sh build containers

distclean: go.distclean clean external.distclean

build.platform.%:
	@$(MAKE) GOOS=$(word 1, $(subst _, ,$*)) GOARCH=$(word 2, $(subst _, ,$*)) build

cross: $(foreach p,$(ALL_PLATFORMS), build.platform.$(p))

release: cross
	@$(MAKE) release.build

publish: release
	@$(MAKE) release.publish

.PHONY: build install test check vet fmt vendor clean distclean cross release publish

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build       Build project for host platform.'
	@echo '    cross       Build project for all platforms.'
	@echo '    check       Runs unit tests.'
	@echo '    clean       Remove all files that are created '
	@echo '                by building.'
	@echo '    dev         A quick build path for go projects'
	@echo '                and containers. Skips building externals.'
	@echo '    distclean   Remove all files that are created '
	@echo '                by building or configuring.'
	@echo '    fmt         Check formatting of go sources.'
	@echo '    help        Show this help info.'
	@echo '    vendor      Installs vendor dependencies.'
	@echo '    vet         Runs lint checks on go sources.'
	@echo ''
	@echo 'Distribution:'
	@echo '    release     Builds all packages.'
	@echo '    publish     Builds and publishes all packages.'
	@echo ''
	@echo 'Options:'
	@echo '    GOARCH      The arch to build.'
	@echo '    PIE         Set to 1 to build build a position independent'
	@echo '                executable. Can not be combined with LINKMODE'
	@echo '                set to "static". The default is 0.'
	@echo '    GOOS        The OS to build for.'
	@echo '    LINKMODE    Set to "dynamic" to link all libraries dynamically.'
	@echo '                Set to "stdlib" to link the standard library'
	@echo '                dynamically and everything else statically. Set to'
	@echo '                "static" to link everything statically. Default is'
	@echo '                "dynamic".'
	@echo '    VERSION     The version information compiled into binaries.'
	@echo '                The default is obtained from git.'
	@echo '    V           Set to 1 enable verbose build. Default is 0.'
	@echo '    WORKDIR     The working directory where most build files'
	@echo '                are stored. The default is .work'
