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

include image.mk

SUPPORTED_PLATFORMS = linux_amd64 linux_arm linux_arm64
PLATFORMS ?= $(SUPPORTED_PLATFORMS)
PLATFORM_IMAGES ?= rookd toolbox

# ====================================================================================
# Image Builds

base.%:
	@$(MAKE) -C base GOARCH=$*

cross-gnu.%: base.%
	@$(MAKE) -C cross-gnu GOARCH=$*

cross.%: base.%
	@$(MAKE) -C cross GOARCH=$*

cross: cross.amd64

ceph.%: cross-gnu.amd64
	@$(MAKE) -C ceph GOARCH=$*

rookd.%: ceph.% base.%
	@$(MAKE) -C rookd GOARCH=$*

toolbox.%: rookd.%
	@$(MAKE) -C toolbox GOARCH=$*

do.build.images:
ifneq ($(filter $(SUPPORTED_PLATFORMS), $(GOOS)_$(GOARCH)),)
	@$(MAKE) $(foreach i,$(PLATFORM_IMAGES), $(i).$(GOARCH))
else
	@echo skipping container build for unspported platform $(GOOS)_$(GOARCH)
endif

do.build.platform.%:
	@$(MAKE) GOOS=$(word 1, $(subst _, ,$*)) GOARCH=$(word 2, $(subst _, ,$*)) do.build.images

do.build: do.build.platform.$(GOOS)_$(GOARCH)

build.all: $(foreach p,$(PLATFORMS), do.build.platform.$(p))

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build        Build images for the host platform.'
	@echo '    build.all    Build images for all platforms.'
	@echo '    clean        Remove all images created from the'
	@echo '                 current build.'
	@echo '    prune        Prunes orphaned and cached images at the.'
	@echo '                 host level.'
	@echo ''
	@echo 'Advanced Targets:'
	@echo '    cross        Build images used for cross building.'
	@echo '    clean.all    Remove all images including cached.'
	@echo '                 images at the host level.'
	@echo '    prune.all    Prune everything ignoring policy.'
	@echo ''
	@echo 'Options:'
	@echo '    PULL         Whether to pull base images. Default is 1.'
	@echo '    GOARCH       The arch to build.'
	@echo '    GOOS         The OS to build for.'
	@echo '    REGISTRY     The docker registry to push to. Default is'
	@echo '                 quay.io/rook.'
	@echo '    V            Set to 1 enable verbose build. Default is 0.'
	@echo ''
	@echo 'Advanced Options:'
	@echo '    PLATFORMS    The supported platforms to build when running.'
	@echo '                 the build.all target. The default is'
	@echo '                 "linux_arm linux_arm64 linux_amd64"'
	@echo '    PRUNE_HOURS  The number of hours from when an image is'
	@echo '                 last used for it to be considered a target for'
	@echo '                 pruning. Default is 48 hrs.'
	@echo '    PRUNE_KEEP   The minimum number of cached images to keep.'
	@echo '                 Default is 24 images.'
