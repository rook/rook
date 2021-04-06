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
CEPH_VERSION = v15.2.9-20210224
else
CEPH_VERSION = v15.2.9-20210224
endif
BASEIMAGE = ceph/ceph-$(GOARCH):$(CEPH_VERSION)
CEPH_IMAGE = $(BUILD_REGISTRY)/ceph-$(GOARCH)
OPERATOR_SDK_VERSION = v0.17.1
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go

TEMP := $(shell mktemp -d)

ifeq ($(HOST_PLATFORM),linux_amd64)
OPERATOR_SDK_PLATFORM = x86_64-linux-gnu
INCLUDE_CSV_TEMPLATES = true
endif

OPERATOR_SDK := $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION)
YQ := $(TOOLS_HOST_DIR)/yq

# ====================================================================================
# Build Rook

do.build: generate-csv-ceph-templates
	@echo === container build $(CEPH_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp toolbox.sh $(TEMP)
	@cp set-ceph-debug-level $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rookflex $(TEMP)
	@cp -r ../../cluster/examples/kubernetes/ceph/csi/template $(TEMP)/ceph-csi
	@cp -r ../../cluster/examples/kubernetes/ceph/monitoring $(TEMP)/ceph-monitoring
	@mkdir -p $(TEMP)/rook-external/test-data
	@cp ../../cluster/examples/kubernetes/ceph/create-external-cluster-resources.* $(TEMP)/rook-external/
	@cp ../../cluster/examples/kubernetes/ceph/test-data/ceph-status-out $(TEMP)/rook-external/test-data/
	@if [ ! "$(INCLUDE_CSV_TEMPLATES)" = "" ]; then\
		cp -r ../../cluster/olm/ceph/templates $(TEMP)/ceph-csv-templates;\
	else\
		mkdir $(TEMP)/ceph-csv-templates;\
	fi
	@cd $(TEMP) && $(SED_CMD) 's|BASEIMAGE|$(BASEIMAGE)|g' Dockerfile
	@$(DOCKERCMD) build $(BUILD_ARGS) \
		--build-arg ARCH=$(GOARCH) \
		--build-arg TINI_VERSION=$(TINI_VERSION) \
		-t $(CEPH_IMAGE) \
		$(TEMP)
	@rm -fr $(TEMP)
	@$(MAKE) -C ../.. crds # revert changes made to the crds.yaml file during the csv-gen sequence

generate-csv-ceph-templates: $(OPERATOR_SDK) $(YQ)
	@if [ ! "$(INCLUDE_CSV_TEMPLATES)" = "" ]; then\
		BEFORE_GEN_CRD_SIZE=$$(wc -l < ../../cluster/examples/kubernetes/ceph/crds.yaml);\
		$(MAKE) -C ../.. NO_OB_OBC_VOL_GEN=true MAX_DESC_LEN=0 crds;\
		AFTER_GEN_CRD_SIZE=$$(wc -l < ../../cluster/examples/kubernetes/ceph/crds.yaml);\
		if [ "$$BEFORE_GEN_CRD_SIZE" -le "$$AFTER_GEN_CRD_SIZE" ]; then\
			echo "the new crd file must be smaller since the description fields were stripped!";\
			echo "length before $$BEFORE_GEN_CRD_SIZE";\
			echo "length after $$AFTER_GEN_CRD_SIZE";\
			exit 1;\
		fi;\
		OPERATOR_SDK=$(OPERATOR_SDK) YQ_TOOL=$(YQ) ../../cluster/olm/ceph/generate-rook-csv-templates.sh;\
	fi

$(YQ):
	@if [ ! "$(INCLUDE_CSV_TEMPLATES)" = "" ]; then\
		echo === installing yq $(GOHOST);\
		curl -JL https://github.com/mikefarah/yq/releases/download/3.3.0/yq_$(HOST_PLATFORM) -o $(YQ);\
		chmod +x $(YQ);\
	fi

$(OPERATOR_SDK):
	@if [ ! "$(INCLUDE_CSV_TEMPLATES)" = "" ]; then\
		curl -JL https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk-$(OPERATOR_SDK_VERSION)-$(OPERATOR_SDK_PLATFORM) -o $(TOOLS_HOST_DIR)/operator-sdk-$(OPERATOR_SDK_VERSION);\
		chmod +x $(OPERATOR_SDK);\
        fi
