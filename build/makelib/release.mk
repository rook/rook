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

# Optional. the platforms to release
RELEASE_PLATFORMS ?=

# Optional. the platforms we release only clients bits
RELEASE_CLIENT_ONLY_PLATFORMS ?=

# Optional. the flavors to release
RELEASE_FLAVORS := binaries containers

# Optional. Github token repo and user
GITHUB_TOKEN ?=
GITHUB_USER ?= rook
GITHUB_REPO ?= rook

export RELEASE_VERSION RELEASE_BIN_DIR RELEASE_DIR
export GITHUB_TOKEN GITHUB_USER GITHUB_REPO

# ====================================================================================
# Targets

define release-target
release.build.$(1).$(2):
	@build/release/release.sh build $(2) $(1)

release.build.all: release.build.$(1).$(2)

release.publish.$(1).$(2):
	@build/release/release.sh publish $(2) $(1)

release.publish.all: release.publish.$(1).$(2)
endef

$(foreach f,$(RELEASE_FLAVORS),$(foreach p,$(RELEASE_PLATFORMS), $(eval $(call release-target,$(f),$(p)))))

release.build: release.build.all

release.publish:
	@build/release/release.sh check
	@$(MAKE) release.publish.all
