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

SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

include $(SELF_DIR)/../build/makelib/cross.mk

# a registry that is scoped to the local host
HOSTNAME := $(shell hostname)
HOST_REGISTRY := host-$(shell echo $(HOSTNAME) | sha256sum | cut -c1-8)

# a registry that is scoped to the current build tree on this host
ROOTDIR=$(shell cd $(SELF_DIR)/.. && pwd -P)
BUILD_REGISTRY := build-$(shell echo $(HOSTNAME)-$(ROOTDIR) | sha256sum | cut -c1-8)

# public registry used for images that are pushed
REGISTRY ?= quay.io/rook

# version to use if not defined
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --dirty --always --tags)
endif

# the base ubuntu image to use
OSBASE ?= ubuntu:zesty

ifeq ($(GOARCH),amd64)
OSBASEIMAGE=$(OSBASE)
endif
ifeq ($(GOARCH),arm)
OSBASEIMAGE=armhf/$(OSBASE)
endif
ifeq ($(GOARCH),arm64)
OSBASEIMAGE=aarch64/$(OSBASE)
endif

UNAME_S:=$(shell uname -s)
ifeq ($(UNAME_S),Darwin)
SED_CMD?=sed -i ""
endif
ifeq ($(UNAME_S),Linux)
SED_CMD?=sed -i
endif

INTERACTIVE:=$(shell [ -t 0 ] && echo 1)
ifdef INTERACTIVE
RUN_ARGS ?= -t
endif

CACHEBUST ?= 0
ifeq ($(CACHEBUST),1)
BUILD_ARGS += --no-cache
endif

V ?= 0
ifeq ($(V),1)
MAKEFLAGS += VERBOSE=1
else
MAKEFLAGS += --no-print-directory
BUILD_ARGS ?= -q
endif

PULL ?= 1
ifeq ($(PULL),1)
BUILD_BASE_ARGS += --pull
endif
export PULL

BUILD_BASE_ARGS += $(BUILD_ARGS)

# prune images that are at least this many hours old
PRUNE_HOURS ?= 48

# prune keeps at least this many images regardless of how old they are
PRUNE_KEEP_CACHED ?= 24
PRUNE_KEEP_ORPHANS ?= 24

CLEANUP_DATE := $(shell date -u --date="$(PRUNE_HOURS) hours ago" +"%Y-%m-%d %T %z UTC")

# NOTE: docker rmi removes the last image when untagging. instead we switch the tag
# to a small dummy orphaning the original image and then call docker rmi. This approach
# avoids removing the last layer (and having to rebuild it)

DUMMY_IMAGE_BASE ?= tianon/true
DUMMY_IMAGE ?= $(HOST_REGISTRY)/dummy:latest

clean.init:
	@if [ -z "`docker images -q $(DUMMY_IMAGE)`" ]; then \
		docker pull $(DUMMY_IMAGE_BASE) > /dev/null 2>&1; \
		docker tag $(DUMMY_IMAGE_BASE) $(DUMMY_IMAGE) > /dev/null 2>&1; \
		docker rmi $(DUMMY_IMAGE_BASE) > /dev/null 2>&1; \
	fi

do.orphan: clean.init
	@for i in $(CLEAN_IMAGES); do \
		[ "$$i" != "$(DUMMY_IMAGE)" ] || continue; \
		for c in $$(docker ps -a -q --filter=ancestor=$$i); do \
			echo stopping and removing container $${c} referencing image $$i; \
			docker stop $${c}; \
			docker rm $${c}; \
		done; \
		echo orphaning image $$i; \
		docker tag $(DUMMY_IMAGE) $$i > /dev/null 2>&1; \
		docker rmi $$i > /dev/null 2>&1; \
	done

# this will clean all images in the variable CLEAN_IMAGES
clean.images: do.clean

# this will clean everything for this build
clean.build: clean.init
	@$(MAKE) do.orphan CLEAN_IMAGES="$(shell docker images | grep -E '^$(BUILD_REGISTRY)|$(REGISTRY)/' | awk '{print $$1":"$$2}')"

# this will clean everything for this host regardless of prune policy
clean.all: clean.init
	@$(MAKE) do.orphan CLEAN_IMAGES="$(shell docker images | grep -E '^$(BUILD_REGISTRY)|$(HOST_REGISTRY)|$(REGISTRY)/' | awk '{print $$1":"$$2}')"

# prune removes old orphaned images and old cached images
prune:
	@echo === pruning images older than $(PRUNE_HOURS) hours
	@echo === keeping a minimum of $(PRUNE_KEEP_ORPHANS) orphaned images and $(PRUNE_KEEP_CACHED) cached images
	@CLEAN_IMAGES="$(shell docker images --format "{{.CreatedAt}}#{{.Repository}}:{{.Tag}}" \
		| grep -E '$(HOST_REGISTRY)/' \
		| grep -v '$(DUMMY_IMAGE)' \
		| sort -r \
		| awk -v i=0 -v cd="$(CLEANUP_DATE)" -F  "#" '{if ($$1 <= cd && i >= $(PRUNE_KEEP_CACHED)) print $$2; i++ }')" &&\
		$(MAKE) do.orphan CLEAN_IMAGES="$${CLEAN_IMAGES}"
	@CLEAN_IMAGES="$(shell docker images -q -f dangling=true --format "{{.CreatedAt}}#{{.ID}}" \
		| sort -r \
		| awk -v i=0 -v cd="$(CLEANUP_DATE)" -F  "#" '{if ($$1 <= cd && i >= $(PRUNE_KEEP_ORPHANS)) print $$2; i++ }')" &&\
		for i in $${CLEAN_IMAGES}; do \
			echo removing orphaned image $$i; \
			docker rmi $$i > /dev/null 2>&1; \
		done

prune.all:
	@for i in $$(docker images -q -f dangling=true); do \
		echo removing orphaned image $$i; \
		docker rmi $$i > /dev/null 2>&1; \
	done
