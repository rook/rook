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

SHELL := /bin/bash

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

GOHOSTOS := $(shell go env GOHOSTOS)
GOHOSTARCH := $(shell go env GOHOSTARCH)
HOST_PLATFORM := $(GOHOSTOS)_$(GOHOSTARCH)

ALL_PLATFORMS ?= darwin_amd64 windows_amd64 linux_amd64 linux_arm64

ifeq ($(PLATFORM),linux_amd64)
CROSS_TRIPLE = x86_64-linux-gnu
endif
ifeq ($(PLATFORM),linux_arm64)
CROSS_TRIPLE = aarch64-linux-gnu
endif
ifeq ($(PLATFORM),darwin_amd64)
CROSS_TRIPLE=x86_64-apple-darwin15
endif
ifeq ($(PLATFORM),windows_amd64)
CROSS_TRIPLE=x86_64-w64-mingw32
endif
export GOARM

ifneq ($(PLATFORM),$(HOST_PLATFORM))
CC := $(CROSS_TRIPLE)-gcc
CXX := $(CROSS_TRIPLE)-g++
export CC CXX
endif

SED_CMD?=sed -i -e

# set the version number. you should not need to do this
# for the majority of scenarios.
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --dirty --always --tags | sed 's/-g/.g/g;s/-dirty/.dirty/g')
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
TOOLS_HOST_DIR := $(TOOLS_DIR)/$(HOST_PLATFORM)

ifeq ($(origin HOSTNAME), undefined)
HOSTNAME := $(shell hostname)
endif

# a registry that is scoped to the current build tree on this host
ifeq ($(origin BUILD_REGISTRY), undefined)
BUILD_REGISTRY := build-$(shell echo $(HOSTNAME)-$(ROOT_DIR) | shasum -a 256 | cut -c1-8)
endif

COMMA := ,
SPACE :=
SPACE +=

# define a newline
define \n


endef
