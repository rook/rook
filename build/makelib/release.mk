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
# Makefile helper functions for distribution

# ====================================================================================
# Configuration

# the release version to use when publishing
RELEASE_VERSION ?=
ifndef RELEASE_VERSION
$(error RELEASE_VERSION must be set before including release.mk)
endif

ifndef BIN_DIR
$(error BIN_DIR must be set before including release.mk)
endif

ifndef RELEASE_DIR
$(error RELEASE_DIR must be set before including release.mk)
endif

# Optional. the platforms we release server and client bits
RELEASE_CLIENT_SERVER_PLATFORMS ?=

# Optional. the platforms we release only clients bits
RELEASE_CLIENT_ONLY_PLATFORMS ?=

# Optional. the host platform
RELEASE_HOST_PLATFORM ?= $(shell go env GOHOSTOS)_$(shell go env GOHOSTARCH)

# Optional. the flavors to release
RELEASE_FLAVORS := binaries containers

# Optional. Github token repo and user
GITHUB_TOKEN ?=
GITHUB_USER ?= rook
GITHUB_REPO ?= rook

export RELEASE_VERSION RELEASE_BIN_DIR RELEASE_DIR
export RELEASE_HOST_PLATFORM RELEASE_CLIENT_SERVER_PLATFORMS RELEASE_CLIENT_ONLY_PLATFORMS
export GITHUB_TOKEN GITHUB_USER GITHUB_REPO

# ====================================================================================
# Targets

define release-flavor
release.build.$(1):
	@build/release/release.sh build $(1)

release.publish.$(1):
	@build/release/release.sh publish $(1)
endef

$(foreach f,$(RELEASE_FLAVORS),$(eval $(call release-flavor,$(f))))

release.build.parallel: $(foreach f,$(RELEASE_FLAVORS), release.build.$(f))

release.publish.parallel: $(foreach f,$(RELEASE_FLAVORS), release.publish.$(f))

release.build:
	@$(MAKE) release.build.parallel

release.publish: release.build
	@build/release/release.sh check
	@$(MAKE) release.publish.parallel
