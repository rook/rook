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

override GOOS=linux

ifeq ($(origin DOCKERCMD),undefined)
DOCKERCMD?=$(shell docker version >/dev/null 2>&1 && echo docker)
ifeq ($(DOCKERCMD),)
DOCKERCMD=$(shell podman version >/dev/null 2>&1 && echo podman)
endif
endif

# include the common make file
SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))
include $(SELF_DIR)/../build/makelib/common.mk

# the registry used for cached images
CACHE_REGISTRY := cache

# the base image to use
OSBASE ?= centos:7

ifeq ($(GOARCH),amd64)
PLATFORM_ARCH = x86_64
OSBASEIMAGE = $(OSBASE)
else ifeq ($(GOARCH),arm64)
PLATFORM_ARCH = aarch64
OSBASEIMAGE = arm64v8/$(OSBASE)
else
$(error Unknown go architecture $(GOARCH))
endif

# if we are running inside the container get our own cid
ifeq ($(UNAME_S),Linux)
SELF_CID := $(shell cat /proc/self/cgroup | grep docker | grep -o -E '[0-9a-f]{64}' | head -n 1)
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

ifeq ($(origin IMAGE_OUTPUT_DIR),undefined)
IMAGE_OUTPUT_DIR := $(OUTPUT_DIR)/images/$(PLATFORM)
endif

BUILD_BASE_ARGS += $(BUILD_ARGS)

# =====================================================================================
# Targets
#
.PHONY: all build publish clean
all: build

build: do.build ## Build images for the host platform.
	@$(MAKE) cache.images

clean: clean.build ## Remove all images created from the current build.

prune: cache.prune ## Prunes orphaned and cached images at the host level.

clean.images:
	@for i in $(CLEAN_IMAGES); do \
		if [ -n "$$($(DOCKERCMD) images -q $$i)" ]; then \
			for c in $$($(DOCKERCMD) ps -a -q --no-trunc --filter=ancestor=$$i); do \
				if [ "$$c" != "$(SELF_CID)" ]; then \
					echo stopping and removing container $${c} referencing image $$i; \
					$(DOCKERCMD) stop $${c}; \
					$(DOCKERCMD) rm $${c}; \
				fi; \
			done; \
			echo cleaning image $$i; \
			$(DOCKERCMD) rmi $$i > /dev/null 2>&1 || true; \
		fi; \
	done

# this will clean everything for this build
clean.build:
	@echo === cleaning images for $(BUILD_REGISTRY)
	@$(MAKE) clean.images CLEAN_IMAGES="$(shell $(DOCKERCMD) images | grep -E '^$(BUILD_REGISTRY)/' | awk '{print $$1":"$$2}')"

# =====================================================================================
# Caching
#

# NOTE: in order to reduce built time, we maintain a cache
# of already built images. This cache contains images that can be used to help speed
# future docker build commands using docker's content addressable schemes. And also
# to avoid running builds like ceph when the contents have not changed.
# All cached images go in in a 'cache/' local registry and we follow an MRU caching
# policy -- keeping images that have been referenced around and evicting images
# that have to been referenced in a while (and according to a policy). Note we can
# not rely on the image's .CreatedAt date since docker only updates then when the
# image is created and not referenced. Instead we keep a date in the Tag.

# prune images that are at least this many hours old
PRUNE_HOURS ?= 48

# prune keeps at least this many images regardless of how old they are
PRUNE_KEEP ?= 24

PRUNE_DRYRUN ?= 0

CACHE_DATE_FORMAT := "%Y-%m-%d.%H%M%S"
CACHE_PRUNE_DATE := $(shell export TZ="UTC+$(PRUNE_HOURS)"; date +"$(CACHE_DATE_FORMAT)")
CACHE_TAG := $(shell date -u +"$(CACHE_DATE_FORMAT)")

cache.lookup:
	@IMAGE_NAME=$${LOOKUP_IMAGE#*/} ;\
	if [ -n "$$($(DOCKERCMD) images -q $(LOOKUP_IMAGE))" ]; then exit 0; fi; \
	if [ -z "$$($(DOCKERCMD) images -q $(CACHE_REGISTRY)/$${IMAGE_NAME})" ]; then \
		$(MAKE) $(MISS_TARGET); \
	else \
		$(DOCKERCMD) tag $$($(DOCKERCMD) images -q $(CACHE_REGISTRY)/$${IMAGE_NAME}) $(LOOKUP_IMAGE); \
	fi;

cache.images:
	@for i in $(CACHE_IMAGES); do \
		IMGID=$$($(DOCKERCMD) images -q $$i); \
		if [ -n "$$IMGID" ]; then \
			echo === caching image $$i; \
			CACHE_IMAGE=$(CACHE_REGISTRY)/$${i#*/}; \
			$(DOCKERCMD) tag $$i $${CACHE_IMAGE}:$(CACHE_TAG); \
			for r in $$($(DOCKERCMD) images --format "{{.ID}}#{{.Repository}}:{{.Tag}}" | grep $$IMGID | grep $(CACHE_REGISTRY)/ | grep -v $${CACHE_IMAGE}:$(CACHE_TAG)); do \
				$(DOCKERCMD) rmi $${r#*#} > /dev/null 2>&1 || true; \
			done; \
		fi; \
	done

# prune removes old cached images
cache.prune:
	@echo === pruning images older than $(PRUNE_HOURS) hours
	@echo === keeping a minimum of $(PRUNE_KEEP) images
	@EXPIRED=$$($(DOCKERCMD) images --format "{{.Tag}}#{{.Repository}}:{{.Tag}}" \
		| grep -E '$(CACHE_REGISTRY)/' \
		| sort -r \
		| awk -v i=0 -v cd="$(CACHE_PRUNE_DATE)" -F  "#" '{if ($$1 <= cd && i >= $(PRUNE_KEEP)) print $$2; i++ }') &&\
	for i in $$EXPIRED; do \
		echo removing expired cache image $$i; \
		[ $(PRUNE_DRYRUN) = 1 ] || $(DOCKERCMD) rmi $$i > /dev/null 2>&1 || true; \
	done
	@for i in $$($(DOCKERCMD) images -q -f dangling=true); do \
		echo removing dangling image $$i; \
		$(DOCKERCMD) rmi $$i > /dev/null 2>&1 || true; \
	done

# =====================================================================================
# Debugging nukes all images
#
debug.nuke:
	@for c in $$($(DOCKERCMD) ps -a -q --no-trunc); do \
		if [ "$$c" != "$(SELF_CID)" ]; then \
			echo stopping and removing container $${c}; \
			$(DOCKERCMD) stop $${c}; \
			$(DOCKERCMD) rm $${c}; \
		fi; \
	done
	@for i in $$($(DOCKERCMD) images -q); do \
		echo removing image $$i; \
		$(DOCKERCMD) rmi -f $$i > /dev/null 2>&1; \
	done
