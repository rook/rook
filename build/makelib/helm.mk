# Copyright 2017 The Rook Authors. All rights reserved.
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

# the helm charts to build
HELM_CHARTS ?= rook-ceph
HELM_BASE_URL ?= https://charts.rook.io
HELM_S3_BUCKET ?= rook.charts
HELM_CHARTS_DIR ?= $(ROOT_DIR)/cluster/charts
HELM_OUTPUT_DIR ?= $(OUTPUT_DIR)/charts

HELM_HOME := $(abspath $(CACHE_DIR)/helm)
HELM_VERSION := v2.5.1
HELM := $(TOOLS_HOST_DIR)/helm-$(HELM_VERSION)
HELM_INDEX := $(HELM_OUTPUT_DIR)/index.yaml
export HELM_HOME

$(HELM_OUTPUT_DIR):
	@mkdir -p $@

$(HELM):
	@echo === installing helm
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://storage.googleapis.com/kubernetes-helm/helm-$(HELM_VERSION)-$(GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/$(GOHOSTOS)-$(GOHOSTARCH)/helm $(HELM)
	@rm -fr $(TOOLS_HOST_DIR)/tmp
	@$(HELM) init -c

define helm.chart
$(HELM_OUTPUT_DIR)/$(1)-$(VERSION).tgz: $(HELM) $(HELM_OUTPUT_DIR) $(shell find $(HELM_CHARTS_DIR)/$(1) -type f)
	@echo === helm package $(1)
	@cp -f $(HELM_CHARTS_DIR)/$(1)/values.yaml.tmpl $(HELM_CHARTS_DIR)/$(1)/values.yaml
	@cd $(HELM_CHARTS_DIR)/$(1) && $(SED_CMD) 's|%%VERSION%%|$(VERSION)|g' values.yaml
	@$(HELM) lint --strict $(abspath $(HELM_CHARTS_DIR)/$(1))
	@$(HELM) package --version $(VERSION) -d $(HELM_OUTPUT_DIR) $(abspath $(HELM_CHARTS_DIR)/$(1))
$(HELM_INDEX): $(HELM_OUTPUT_DIR)/$(1)-$(VERSION).tgz
endef
$(foreach p,$(HELM_CHARTS),$(eval $(call helm.chart,$(p))))

$(HELM_INDEX): $(HELM) $(HELM_OUTPUT_DIR)
	@echo === helm index
	@$(HELM) repo index $(HELM_OUTPUT_DIR)

helm.build: $(HELM_INDEX)
