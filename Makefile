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

# set to 1 for a completely static build. Otherwise if set to 0
# a dynamic binary is produced that requires glibc to be installed
STATIC ?= 1
ifeq ($(STATIC),1)
TAGS += static
else
TAGS += dynamic
endif

# build a position independent executable. This implies dynamic linking
# since statically-linked PIE is not supported by the linker/glibc. PIE
# is only supported on Linux.
PIE ?= 0
ifeq ($(PIE),1)
ifeq ($(STATIC),1)
$(error PIE only supported with dynamic linking. Set STATIC=0.)
endif
endif

# if DEBUG is set to 1 debug information is perserved (i.e. not stripped).
# the binary size is going to be much larger.
DEBUG ?= 0
ifeq ($(DEBUG),0)
LDFLAGS += -w
endif

# the memory allocator to use for cephd
ALLOCATOR ?= tcmalloc_minimal

# whether to use ccache when building cephd
CCACHE ?= 1

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v
BUILDFLAGS += -x
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

CLIENT_SERVER_PLATFORMS ?= linux_amd64 linux_arm64
CLIENT_ONLY_PLATFORMS ?= darwin_amd64 windows_amd64
ALL_PLATFORMS ?= $(CLIENT_SERVER_PLATFORMS) $(CLIENT_ONLY_PLATFORMS)

GO_PROJECT=github.com/rook/rook

# set the version number.
ifeq ($(origin VERSION), undefined)
VERSION = $(shell git describe --dirty --always)
endif
LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)

# ====================================================================================
# Setup rookd

# support for cross compiling
include build/makelib/cross.mk

ifeq ($(GOOS)_$(GOARCH),linux_amd64)
ROOKD_SUPPORTED := 1
endif

ifeq ($(GOOS)_$(GOARCH),linux_arm64)
ROOKD_SUPPORTED := 1
endif

ifeq ($(ROOKD_SUPPORTED),1)

CEPHD_DEBUG = $(DEBUG)
CEPHD_CCACHE = $(CCACHE)
CEPHD_ALLOCATOR = $(ALLOCATOR)
CEPHD_BUILD_DIR = $(WORKDIR)/ceph
CEPHD_PLATFORM = $(GOOS)_$(GOARCH)

# go does not check dependencies listed in LDFLAGS. we touch the dummy source file
# to force go to rebuild cephd
CEPHD_TOUCH_ON_BUILD = pkg/cephmgr/cephd/dummy.cc

CGO_LDFLAGS = -L$(abspath $(CEPHD_BUILD_DIR)/$(CEPHD_PLATFORM)/lib)
CGO_PREREQS = cephd.build

include build/makelib/cephd.mk

clean: cephd.clean

endif

# ====================================================================================
# Setup Go projects

GO_WORK_DIR = $(WORKDIR)
GO_BIN_DIR = $(BIN_DIR)

ifeq ($(STATIC),1)
GO_STATIC_PACKAGES=$(GO_PROJECT)
ifeq ($(ROOKD_SUPPORTED),1)
GO_STATIC_CGO_PACKAGES=$(GO_PROJECT)/cmd/rookd
endif
else
GO_NONSTATIC_PACKAGES=$(GO_PROJECT)
ifeq ($(ROOKD_SUPPORTED),1)
ifeq ($(PIE),1)
GO_NONSTATIC_PIE_PACKAGES+=$(GO_PROJECT)/cmd/rookd
else
GO_NONSTATIC_PACKAGES+= $(GO_PROJECT)/cmd/rookd
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
RELEASE_CLIENT_SERVER_PLATFORMS=$(CLIENT_SERVER_PLATFORMS)
RELEASE_CLIENT_ONLY_PLATFORMS=$(CLIENT_ONLY_PLATFORMS)
include build/makelib/release.mk

# ====================================================================================
# Targets

build: go.build

install: go.install

check test: go.test

vet: go.vet

fmt: go.fmt

vendor: go.vendor

clean: go.clean
	@rm -fr $(WORKDIR) $(RELEASE_DIR)/* $(BIN_DIR)/*

distclean: go.distclean clean

build.platform.%:
	@$(MAKE) GOOS=$(word 1, $(subst _, ,$*)) GOARCH=$(word 2, $(subst _, ,$*)) build

build.cross: $(foreach p,$(ALL_PLATFORMS), build.platform.$(p))

cross:
	@$(MAKE) build.cross

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
	@echo ''
	@echo '    GOARCH      The arch to build.'
	@echo '    CCACHE      Set to 1 to enabled ccache, 0 to disable.'
	@echo '                The default is 0.'
	@echo '    DEBUG       Set to 1 to disable stripping the binaries of.'
	@echo '                debug information. The default is 0.'
	@echo '    PIE         Set to 1 to build build a position independent'
	@echo '                executable. Can not be combined with static.'
	@echo '                The default is 0.'
	@echo '    GOOS        The OS to build for.'
	@echo '    STATIC      Set to 1 for static build, 0 for dynamic.'
	@echo '                The default is 1.'
	@echo '    VERSION     The version information compiled into binaries.'
	@echo '                The default is obtained from git.'
	@echo '    V           Set to 1 enable verbose build. Default is 0.'
	@echo '    WORKDIR     The working directory where most build files'
	@echo '                are stored. The default is .work'
