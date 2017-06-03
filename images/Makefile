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

.PHONY: all build push clean
all: build

include image.mk

PLATFORMS ?= amd64 arm arm64

.PHONY: cross-build
cross-build:
	@$(MAKE) -C base GOARCH=amd64
	@$(MAKE) -C cross-build-gnu
	@$(MAKE) -C cross-build-go
	@$(MAKE) -C cross-build

build.%:
	@$(MAKE) -C base build GOARCH=$*
	@$(MAKE) -C rookd build GOARCH=$*

build:
	@$(MAKE) -C base GOARCH=amd64
	@$(MAKE) -C cross-build-gnu
	@$(MAKE) build.$(GOARCH)

push.%:
	@$(MAKE) -C rookd push GOARCH=$*
push: $(foreach p,$(PLATFORMS), push.$(p))

clean: clean.build

cross.all: $(foreach p,$(PLATFORMS), build.$(p))
cross:
	@$(MAKE) -C base GOARCH=amd64
	@$(MAKE) -C cross-build-gnu
	@$(MAKE) cross.all

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build        Build images for the host platform.'
	@echo '    clean        Remove all images created from the'
	@echo '                 current build.'
	@echo '    cross        Build images for all platforms.'
	@echo '    prune        Prunes orphaned and cached images at the.'
	@echo '                 host level.'
	@echo '    push         Pushes public images to registry.'
	@echo ''
	@echo 'Advanced Targets:'
	@echo '    cross-build  Build images used for cross-builds.'
	@echo '    clean.all    Remove all images including cached.'
	@echo '                 images at the host level.'
	@echo '    prune.all    Prune everything ignoring policy.'
	@echo ''
	@echo 'Options:'
	@echo '    PLATFORMS    The platforms to build when running the.'
	@echo '                 cross target. Default is "arm arm64 amd64"'
	@echo '    PULL         Whether to pull base images. Default is 1.'
	@echo '    REGISTRY     The docker registry to push to. Default is'
	@echo '                 quay.io/rook.'
	@echo '    V            Set to 1 enable verbose build. Default is 0.'
	@echo '    VERSION      The version used for image tags. The default'
	@echo '                 is obtained from git.'
	@echo ''
	@echo 'Advanced Options:'
	@echo '    PRUNE_HOURS         The number of hours from when an image is'
	@echo '                        created for it to be considered a target for'
	@echo '                        pruning. Default is 48 hrs.'
	@echo '    PRUNE_KEEP_CACHED   The minimum number of cached images to keep.'
	@echo '                        Default is 24 images.'
	@echo '    PRUNE_KEEP_ORPHANS  The minimum number of orphaned images to keep.'
	@echo '                        Default is 24 images.'
