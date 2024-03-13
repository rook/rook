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

ifeq ($(GOARCH),amd64)
CEPH_VERSION ?= v18.2.2-20240311
else
CEPH_VERSION ?= v18.2.2-20240311
endif
REGISTRY_NAME = quay.io
BASEIMAGE = $(REGISTRY_NAME)/ceph/ceph-$(GOARCH):$(CEPH_VERSION)
CEPH_IMAGE = $(BUILD_REGISTRY)/ceph-$(GOARCH)
OPERATOR_SDK_VERSION = v1.25.0
# TODO: update to yq v4 - v3 end of life in Aug 2021 ; v4 removes the 'yq delete' cmd and changes syntax
YQv3_VERSION = 3.4.1
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go
MANIFESTS_DIR=../../deploy/examples

ifeq ($(BUILD_CONTEXT_DIR),)
BUILD_CONTEXT_DIR := $(shell mktemp -d)
endif

# Note: as of version 1.3 of operator-sdk, the url format changed to:
# ${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH}
# (see: https://sdk.operatorframework.io/docs/installation/)
S5CMD_ARCH = Linux-64bit
ifeq ($(REAL_HOST_PLATFORM),linux_amd64)
OPERATOR_SDK_PLATFORM = x86_64-linux-gnu
INCLUDE_CSV_TEMPLATES = true
endif
ifeq ($(REAL_HOST_PLATFORM),linux_arm64)
OPERATOR_SDK_PLATFORM = aarch64-linux-gnu
INCLUDE_CSV_TEMPLATES = true
S5CMD_ARCH = Linux-arm64
endif
ifeq ($(REAL_HOST_PLATFORM),darwin_amd64)
OPERATOR_SDK_PLATFORM = x86_64-apple-darwin
INCLUDE_CSV_TEMPLATES = true
endif
ifneq ($(INCLUDE_CSV_TEMPLATES),true)
$(info )
$(info NOT INCLUDING OLM/CSV TEMPLATES!)
$(info )
endif

# s5cmd's version
S5CMD_VERSION = 2.2.1

OPERATOR_SDK := $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION)
YQv3 := $(TOOLS_HOST_DIR)/yq-$(YQv3_VERSION)
export OPERATOR_SDK YQv3

# ====================================================================================
# Build Rook

do.build:
	@echo === container build $(CEPH_IMAGE)
	@mkdir -p $(BUILD_CONTEXT_DIR)
	@cp Dockerfile $(BUILD_CONTEXT_DIR)
	@cp toolbox.sh $(BUILD_CONTEXT_DIR)
	@cp set-ceph-debug-level $(BUILD_CONTEXT_DIR)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(BUILD_CONTEXT_DIR)
	@cp -r $(MANIFESTS_DIR)/monitoring $(BUILD_CONTEXT_DIR)/ceph-monitoring
	@mkdir -p $(BUILD_CONTEXT_DIR)/rook-external/test-data
	@cp $(MANIFESTS_DIR)/create-external-cluster-resources.* $(BUILD_CONTEXT_DIR)/rook-external/
	@cp ../../tests/ceph-status-out $(BUILD_CONTEXT_DIR)/rook-external/test-data/

ifeq ($(INCLUDE_CSV_TEMPLATES),true)
	@$(MAKE) csv
	@cp -r ../../build/csv $(BUILD_CONTEXT_DIR)/ceph-csv-templates
	@rm $(BUILD_CONTEXT_DIR)/ceph-csv-templates/csv-gen.sh
	@$(MAKE) csv-clean

else
	mkdir $(BUILD_CONTEXT_DIR)/ceph-csv-templates
endif
	@cd $(BUILD_CONTEXT_DIR) && $(SED_IN_PLACE) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@if [ -z "$(BUILD_CONTAINER_IMAGE)" ]; then\
		$(DOCKERCMD) build $(BUILD_ARGS) \
		--build-arg S5CMD_VERSION=$(S5CMD_VERSION) \
		--build-arg S5CMD_ARCH=$(S5CMD_ARCH) \
		-t $(CEPH_IMAGE) \
		$(BUILD_CONTEXT_DIR);\
	fi
	@if [ -z "$(SAVE_BUILD_CONTEXT_DIR)" ]; then\
		rm -fr $(BUILD_CONTEXT_DIR);\
	fi

# call this before building multiple arches in parallel to prevent parallel build processes from
# conflicting
prerequisites: $(OPERATOR_SDK) $(YQv3)

$(YQv3):
	@echo === installing yq $(YQv3_VERSION) $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL https://github.com/mikefarah/yq/releases/download/$(YQv3_VERSION)/yq_$(REAL_HOST_PLATFORM) -o $(YQv3)
	@chmod +x $(YQv3)

$(OPERATOR_SDK):
	@echo === installing operator-sdk $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL -o $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION) \
		https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(REAL_HOST_PLATFORM)
	@chmod +x $(OPERATOR_SDK)
	@$(OPERATOR_SDK) version

csv: $(OPERATOR_SDK) $(YQv3)
	@echo generate csv with latest operator-sdk
	@mkdir -p ../../build/csv/ceph
	@../../build/csv/csv-gen.sh
	@# #adding 2>/dev/null since CI doesn't seems to be creating bundle.Dockerfile file
	@rm bundle.Dockerfile 2> /dev/null || true

csv-clean: ## Remove existing OLM files.
	@rm -fr ../../build/csv/ceph/${go env GOARCH}
	@rm -f ../../build/csv/rook-ceph.clusterserviceversion.yaml
	@git restore $(MANIFESTS_DIR)/crds.yaml ../../deploy/charts/rook-ceph/templates/resources.yaml

# reading from a file and outputting to the same file can have undefined results, so use this intermediate
IMAGE_TMP="/tmp/rook-ceph-image-list"
list-image: ## Create a list of images for offline installation
	@echo "producing list of images for offline installation"
	rm -f $(IMAGE_TMP)
	awk '/image:/ {print $2}' $(MANIFESTS_DIR)/operator.yaml $(MANIFESTS_DIR)/cluster.yaml | \
		cut -d: -f2- | tee $(IMAGE_TMP)
	awk '/quay.io/ || /registry.k8s.io/ {print $3}' ../../pkg/operator/ceph/csi/spec.go | \
		cut -d= -f2- | tr -d '"' | tee -a $(IMAGE_TMP)
	awk '/quay.io/ || /gcr.io/ {print $4}' ../../pkg/operator/ceph/object/cosi/spec.go | \
		cut -d= -f2- | tr -d '"' | tee -a $(IMAGE_TMP)
	rm -f $(MANIFESTS_DIR)/images.txt
	cat $(IMAGE_TMP) | sort -h | uniq | tee $(MANIFESTS_DIR)/images.txt
	rm -f $(IMAGE_TMP)
