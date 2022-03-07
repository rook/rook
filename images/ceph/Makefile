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
CEPH_VERSION = v16.2.7-20220216
else
CEPH_VERSION = v16.2.7-20220216
endif
REGISTRY_NAME = quay.io
BASEIMAGE = $(REGISTRY_NAME)/ceph/ceph-$(GOARCH):$(CEPH_VERSION)
CEPH_IMAGE = $(BUILD_REGISTRY)/ceph-$(GOARCH)
OPERATOR_SDK_VERSION = v0.17.1
# TODO: update to yq v4 - v3 end of life in Aug 2021 ; v4 removes the 'yq delete' cmd and changes syntax
YQv3_VERSION = 3.3.0
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go
MANIFESTS_DIR=../../deploy/examples

TEMP := $(shell mktemp -d)

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
S5CMD_VERSION = 1.4.0

OPERATOR_SDK := $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION)
YQv3 := $(TOOLS_HOST_DIR)/yq-$(YQv3_VERSION)
export OPERATOR_SDK YQv3

# ====================================================================================
# Build Rook

do.build:
	@echo === container build $(CEPH_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp toolbox.sh $(TEMP)
	@cp set-ceph-debug-level $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@cp -r $(MANIFESTS_DIR)/monitoring $(TEMP)/ceph-monitoring
	@mkdir -p $(TEMP)/rook-external/test-data
	@cp $(MANIFESTS_DIR)/create-external-cluster-resources.* $(TEMP)/rook-external/
	@cp ../../tests/ceph-status-out $(TEMP)/rook-external/test-data/

ifeq ($(INCLUDE_CSV_TEMPLATES),true)
	@$(MAKE) CSV_TEMPLATE_DIR=$(TEMP) generate-csv-templates
	@cp -r $(TEMP)/deploy/olm/templates $(TEMP)/ceph-csv-templates
else
	mkdir $(TEMP)/ceph-csv-templates
endif
	@cd $(TEMP) && $(SED_IN_PLACE) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@if [ -z "$(BUILD_CONTAINER_IMAGE)" ]; then\
		$(DOCKERCMD) build $(BUILD_ARGS) \
		--build-arg S5CMD_VERSION=$(S5CMD_VERSION) \
		--build-arg S5CMD_ARCH=$(S5CMD_ARCH) \
		-t $(CEPH_IMAGE) \
		$(TEMP);\
	fi
	@rm -fr $(TEMP)

# call this before building multiple arches in parallel to prevent parallel build processes from
# conflicting
prerequisites: $(OPERATOR_SDK) $(YQv3)

# generate CSV template files into the directory defined by the env var CSV_TEMPLATE_DIR
# CSV_TEMPLATE_DIR will be created if it doesn't already exist
generate-csv-templates: $(OPERATOR_SDK) $(YQv3) ## Generate CSV templates for OLM into CSV_TEMPLATE_DIR
	@if [[ -z "$(CSV_TEMPLATE_DIR)" ]]; then echo "CSV_TEMPLATE_DIR is not set"; exit 1; fi
	@# first, copy the existing CRDs and OLM catalog directory to CSV_TEMPLATE_DIR
	@# then, generate or copy all prerequisites into CSV_TEMPLATE_DIR (e.g., CRDs)
	@# finally, generate the templates in-place using CSV_TEMPLATE_DIR as a staging dir
	@mkdir -p $(CSV_TEMPLATE_DIR)
	@cp -a ../../deploy $(CSV_TEMPLATE_DIR)/deploy
	@set -eE;\
	BEFORE_GEN_CRD_SIZE=$$(wc -l < $(MANIFESTS_DIR)/crds.yaml);\
	$(MAKE) -C ../.. NO_OB_OBC_VOL_GEN=true MAX_DESC_LEN=0 BUILD_CRDS_INTO_DIR=$(CSV_TEMPLATE_DIR) crds;\
	AFTER_GEN_CRD_SIZE=$$(wc -l < $(CSV_TEMPLATE_DIR)/deploy/examples/crds.yaml);\
	if [ "$$BEFORE_GEN_CRD_SIZE" -le "$$AFTER_GEN_CRD_SIZE" ]; then\
		echo "the new crd file must be smaller since the description fields were stripped!";\
		echo "length before $$BEFORE_GEN_CRD_SIZE";\
		echo "length after $$AFTER_GEN_CRD_SIZE";\
		exit 1;\
	fi
	@OLM_CATALOG_DIR=$(CSV_TEMPLATE_DIR)/deploy/olm ../../deploy/olm/generate-rook-csv-templates.sh
	@echo " === Generated CSV templates can be found at $(CSV_TEMPLATE_DIR)/deploy/olm/templates"

$(YQv3):
	@echo === installing yq $(YQv3_VERSION) $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL https://github.com/mikefarah/yq/releases/download/$(YQv3_VERSION)/yq_$(REAL_HOST_PLATFORM) -o $(YQv3)
	@chmod +x $(YQv3)

$(OPERATOR_SDK):
	@echo === installing operator-sdk $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL -o $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION) \
		https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk-$(OPERATOR_SDK_VERSION)-$(OPERATOR_SDK_PLATFORM)
	@chmod +x $(OPERATOR_SDK)
	@$(OPERATOR_SDK) version

csv: $(OPERATOR_SDK) $(YQv3) $(HELM) ## Generate a CSV file for OLM.
	@echo Generating CSV manifests
	@cd ../.. && deploy/olm/generate-rook-csv.sh $(CSV_VERSION) $(CSV_PLATFORM) $(ROOK_OP_VERSION)

csv-clean: ## Remove existing OLM files.
	@rm -fr ../../deploy/olm/deploy/* ../../deploy/olm/templates/*

# reading from a file and outputting to the same file can have undefined results, so use this intermediate
IMAGE_TMP="/tmp/rook-ceph-image-list"
list-image: ## Create a list of images for offline installation
	@echo "producing list of images for offline installation"
	rm -f $(IMAGE_TMP)
	awk '/image:/ {print $2}' $(MANIFESTS_DIR)/operator.yaml $(MANIFESTS_DIR)/cluster.yaml | \
		cut -d: -f2- | tee $(IMAGE_TMP)
	awk '/quay.io/ || /k8s.gcr.io/ {print $3}' ../../pkg/operator/ceph/csi/spec.go | \
		cut -d= -f2- | tr -d '"' | tee -a $(IMAGE_TMP)
	rm -f $(MANIFESTS_DIR)/images.txt
	cat $(IMAGE_TMP) | sort -h | uniq | tee $(MANIFESTS_DIR)/images.txt
	rm -f $(IMAGE_TMP)
