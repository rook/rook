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

include ../image.mk

# ====================================================================================
# Image Build Options

CEPH_IMAGE = $(BUILD_REGISTRY)/ceph-$(GOARCH)

CEPH_VERSION = v14.2.1-20190430
BASEIMAGE = ceph/ceph-$(GOARCH):$(CEPH_VERSION)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Rook

do.build:
	@echo === docker build $(CEPH_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp toolbox.sh $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rookflex $(TEMP)
	@cp -r ../../cluster/examples/kubernetes/ceph/csi/template $(TEMP)/ceph-csi
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@$(DOCKERCMD) build $(BUILD_ARGS) \
		--build-arg ARCH=$(GOARCH) \
		--build-arg TINI_VERSION=$(TINI_VERSION) \
		-t $(CEPH_IMAGE) \
		$(TEMP)
	@rm -fr $(TEMP)
