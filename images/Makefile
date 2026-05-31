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

PLATFORMS ?= linux_amd64 linux_arm64

# tini's version
export TINI_VERSION = v0.19.0

# ====================================================================================
# Image Targets

ceph.%:
	@$(MAKE) -C ceph PLATFORM=$*

do.build: ceph.$(PLATFORM) ;
build.all: ## Build images for all platforms.
	$(MAKE) -C ceph prerequisites
	$(MAKE) $(foreach p,$(PLATFORMS), ceph.$(p)) ;

# ====================================================================================
# Help
define HELPTEXT
Options:
    PLATFORM     The platform to build.
    PULL         Whether to pull base images. Default is 1.
    V            Set to 1 enable verbose build. Default is 0.

Advanced Options:
    PRUNE_HOURS  The number of hours from when an image is
                 last used for it to be considered a target for
                 pruning. Default is 48 hrs.
    PRUNE_KEEP   The minimum number of cached images to keep.
                 Default is 24 images.
endef
export HELPTEXT
.PHONY: help
help: ## Show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@grep --no-filename -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "$$HELPTEXT"
