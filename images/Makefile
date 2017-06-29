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

PLATFORMS ?= amd64 arm arm64

# ====================================================================================
# Image Targets

apt-cacher: apt-cacher.$(GOHOSTARCH)
apt-cacher.%: base.%
	@$(MAKE) -C apt-cacher GOARCH=$*

base.%:
	@$(MAKE) -C base GOARCH=$*

ceph.%: cross-gnu.$(GOHOSTARCH)
	@$(MAKE) -C ceph GOARCH=$*

cross: cross.$(GOHOSTARCH)
cross.%: base.$(GOHOSTARCH)
	@$(MAKE) -C cross GOARCH=$*

cross-gnu.%: base.%
	@$(MAKE) -C cross-gnu GOARCH=$*

rook.%: ceph.% base.%
	@$(MAKE) -C rook GOARCH=$*

toolbox.%: rook.%
	@$(MAKE) -C toolbox GOARCH=$*

do.build.images.%: rook.% toolbox.% ;
do.build: do.build.images.$(GOARCH) ;
build.all: $(foreach p,$(PLATFORMS), do.build.images.$(p)) ;

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
	@echo '    V            Set to 1 enable verbose build. Default is 0.'
	@echo ''
	@echo 'Advanced Options:'
	@echo '    PLATFORMS    The supported platforms to build when running.'
	@echo '                 the build.all target. The default is'
	@echo '                 "arm arm64 amd64"'
	@echo '    PRUNE_HOURS  The number of hours from when an image is'
	@echo '                 last used for it to be considered a target for'
	@echo '                 pruning. Default is 48 hrs.'
	@echo '    PRUNE_KEEP   The minimum number of cached images to keep.'
	@echo '                 Default is 24 images.'
