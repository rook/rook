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

# remove default suffixes as we dont use them
.SUFFIXES:

SHELL := /usr/bin/env bash
ifneq (, $(shell command -v shasum))
SHA256CMD := shasum -a 256
else ifneq (, $(shell command -v sha256sum))
SHA256CMD := sha256sum
else
$(error "please install 'shasum' or 'sha256sum'")
endif

ifneq ($(strip $(CT_VERSION)),)
CT := go run github.com/helm/chart-testing/v3/ct@$(CT_VERSION)
endif

ifeq ($(origin DOCKERCMD),undefined)
DOCKERCMD?=$(shell docker version >/dev/null 2>&1 && echo docker)
ifeq ($(DOCKERCMD),)
DOCKERCMD=$(shell podman version >/dev/null 2>&1 && echo podman)
endif
endif

MARKDOWNLINT := $(DOCKERCMD) run --rm -v $$PWD:/workdir davidanson/markdownlint-cli2:$(MARKDOWNLINT_IMAGE_VERSION)

ifeq ($(origin PLATFORM), undefined)
ifeq ($(origin GOOS), undefined)
GOOS := $(shell go env GOOS)
endif
ifeq ($(origin GOARCH), undefined)
GOARCH := $(shell go env GOARCH)
endif
PLATFORM := $(GOOS)_$(GOARCH)
else
GOOS := $(word 1, $(subst _, ,$(PLATFORM)))
GOARCH := $(word 2, $(subst _, ,$(PLATFORM)))
export GOOS GOARCH
endif

ALL_PLATFORMS ?= darwin_amd64 darwin_arm64 windows_amd64 linux_amd64 linux_arm64

export GOARM

# force the build of a linux binary when running on MacOS
GOHOSTOS=linux
GOHOSTARCH := $(shell go env GOHOSTARCH)
HOST_PLATFORM := $(GOHOSTOS)_$(GOHOSTARCH)

# REAL_HOST_PLATFORM is used to determine the correct url to download the various binary tools from and it does not use
# HOST_PLATFORM which is used to build the program.
REAL_HOST_PLATFORM=$(shell go env GOHOSTOS)_$(GOHOSTARCH)

# set the version number. you should not need to do this
# for the majority of scenarios.
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --dirty --always --tags | sed 's/-/./2' | sed 's/-/./2' )
endif
export VERSION

# include the common make file
COMMON_SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

ifeq ($(origin ROOT_DIR),undefined)
ROOT_DIR := $(abspath $(shell cd $(COMMON_SELF_DIR)/../.. && pwd -P))
endif
ifeq ($(origin OUTPUT_DIR),undefined)
OUTPUT_DIR := $(ROOT_DIR)/_output
endif
ifeq ($(origin WORK_DIR), undefined)
WORK_DIR := $(ROOT_DIR)/.work
endif
ifeq ($(origin CACHE_DIR), undefined)
CACHE_DIR := $(ROOT_DIR)/.cache
endif

TOOLS_DIR := $(CACHE_DIR)/tools
TOOLS_HOST_DIR := $(TOOLS_DIR)/$(REAL_HOST_PLATFORM)

ifeq ($(origin HOSTNAME), undefined)
HOSTNAME := $(shell hostname)
endif

ifneq ($(strip $(KUSTOMIZE_VERSION)),)
KUSTOMIZE := $(TOOLS_HOST_DIR)/kustomize-$(KUSTOMIZE_VERSION)
endif

$(KUSTOMIZE): | $(TOOLS_HOST_DIR)
	@echo === installing kustomize
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@# kustomize releases use a specific naming convention: kustomize_vX.Y.Z_os_arch.tar.gz
	@curl -sSL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F$(KUSTOMIZE_VERSION)/kustomize_$(KUSTOMIZE_VERSION)_$(shell go env GOHOSTOS)_$(shell go env GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv -f $(TOOLS_HOST_DIR)/tmp/kustomize $(KUSTOMIZE)
	@rm -rf $(TOOLS_HOST_DIR)/tmp
	@chmod +x $(KUSTOMIZE)

# a registry that is scoped to the current build tree on this host
ifeq ($(origin BUILD_REGISTRY), undefined)
BUILD_REGISTRY := build-$(shell echo "$(HOSTNAME)-$(ROOT_DIR)" | $(SHA256CMD) | cut -c1-8)
endif
ifeq ($(BUILD_REGISTRY),build-)
$(error Failed to get unique ID for host+dir. Check that '$(SHA256CMD)' functions or override SHA256CMD)
endif

SED_IN_PLACE = $(ROOT_DIR)/build/sed-in-place
export SED_IN_PLACE

# This is a neat little target that prints any variable value from the Makefile
# Usage: make echo.PLATFORM
echo.%: ; @echo $* = $($*)

# Target for creating the build tools directory
$(TOOLS_HOST_DIR):
	@mkdir -p $@

COMMA := ,
SPACE :=
SPACE +=

# define a newline
define NEWLINE

endef
