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
HELM_CHARTS ?= rook-ceph rook-ceph-cluster
HELM_BASE_URL ?= https://charts.rook.io
HELM_S3_BUCKET ?= rook.chart
HELM_CHARTS_DIR ?= $(ROOT_DIR)/deploy/charts
HELM_OUTPUT_DIR ?= $(OUTPUT_DIR)/charts

# Check if helm chart dependencies need to be downloaded
# For rook-ceph: extracts ceph-csi-operator version from Chart.yaml and checks if .tgz exists
define helm_needs_dependencies
$(shell \
	if [ "$(1)" = "rook-ceph" ]; then \
		version=$$(grep -A1 "name: ceph-csi-operator" $(HELM_CHARTS_DIR)/$(1)/Chart.yaml | grep "version:" | awk '{print $$2}'); \
		if [ ! -f "$(HELM_CHARTS_DIR)/$(1)/charts/ceph-csi-operator-$$version.tgz" ]; then \
			echo "missing"; \
		else \
			echo "present"; \
		fi; \
	fi \
)
endef

HELM_HOME := $(abspath $(CACHE_DIR)/helm)
HELM_VERSION := v3.18.2
HELM := $(TOOLS_HOST_DIR)/helm-$(HELM_VERSION)
HELM_INDEX := $(HELM_OUTPUT_DIR)/index.yaml
export HELM_HOME

$(HELM_OUTPUT_DIR):
	@mkdir -p $@

$(HELM):
	@echo === installing helm
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://get.helm.sh/helm-$(HELM_VERSION)-$(shell go env GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/$(shell go env GOHOSTOS)-$(GOHOSTARCH)/helm $(HELM)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

define helm.chart
$(HELM_OUTPUT_DIR)/$(1)-$(VERSION).tgz: $(HELM) $(HELM_OUTPUT_DIR) helm.dependency.update.$(1) $(shell find $(HELM_CHARTS_DIR)/$(1) -type f)
	@echo === helm package $(1)
	@rm -rf $(OUTPUT_DIR)/$(1)
	@cp -aL $(HELM_CHARTS_DIR)/$(1) $(OUTPUT_DIR)
	@$(SED_IN_PLACE) 's|master|$(VERSION)|g' $(OUTPUT_DIR)/$(1)/values.yaml
	@$(HELM) lint $(abspath $(OUTPUT_DIR)/$(1)) --set image.tag=$(VERSION)
	@$(HELM) package --version $(VERSION) --app-version $(VERSION) -d $(HELM_OUTPUT_DIR) $(abspath $(OUTPUT_DIR)/$(1))
$(HELM_INDEX): $(HELM_OUTPUT_DIR)/$(1)-$(VERSION).tgz
endef
$(foreach p,$(HELM_CHARTS),$(eval $(call helm.chart,$(p))))

$(HELM_INDEX): $(HELM) $(HELM_OUTPUT_DIR)
	@echo === helm index
	@$(HELM) repo index $(HELM_OUTPUT_DIR)

helm.build: $(HELM_INDEX)

# Update helm chart dependencies
define helm.dependency.update
helm.dependency.update.$(1): $(HELM)
	@echo === updating helm dependencies for $(1)
	@if [ "$(call helm_needs_dependencies,$(1))" = "missing" ]; then \
		echo "Dependencies missing, downloading..."; \
		if [ "$(1)" = "rook-ceph" ]; then \
			$(HELM) repo add ceph-csi-operator https://ceph.github.io/ceph-csi-operator --force-update; \
		fi; \
		cd $(HELM_CHARTS_DIR)/$(1) && $(HELM) dependency update; \
	else \
		echo "Dependencies already available, skipping download"; \
	fi
helm.dependency.update: helm.dependency.update.$(1)
endef
$(foreach p,$(HELM_CHARTS),$(eval $(call helm.dependency.update,$(p))))

# Build helm chart dependencies
define helm.dependency.build
helm.dependency.build.$(1): helm.dependency.update.$(1)
	@echo === building helm dependencies for $(1)
	@if [ -f "$(HELM_CHARTS_DIR)/$(1)/Chart.lock" ] && [ "$(HELM_CHARTS_DIR)/$(1)/Chart.lock" -nt "$(HELM_CHARTS_DIR)/$(1)/Chart.yaml" ]; then \
		echo "Chart.lock is up-to-date, skipping dependency build"; \
	else \
		echo "Chart.lock is missing or outdated, running dependency build"; \
		cd $(HELM_CHARTS_DIR)/$(1) && $(HELM) dependency build; \
	fi
helm.dependency.build: helm.dependency.build.$(1)
endef
$(foreach p,$(HELM_CHARTS),$(eval $(call helm.dependency.build,$(p))))

# Clean up helm dependency artifacts from source directories
define helm.dependency.clean
helm.dependency.clean.$(1):
	@echo === cleaning up dependency artifacts for $(1)
	@rm -f $(HELM_CHARTS_DIR)/$(1)/charts/*.tgz
helm.dependency.clean: helm.dependency.clean.$(1)
endef
$(foreach p,$(HELM_CHARTS),$(eval $(call helm.dependency.clean,$(p))))

.PHONY: helm.dependency.update helm.dependency.build helm.dependency.clean
helm.dependency.update: ## Update helm chart dependencies when missing
helm.dependency.build: ## Build helm chart dependencies
helm.dependency.clean: ## Remove downloaded dependency files

# ====================================================================================
# Makefile helper functions for helm-docs: https://github.com/norwoodj/helm-docs
#

HELM_DOCS_VERSION := v1.11.0
HELM_DOCS := $(TOOLS_HOST_DIR)/helm-docs-$(HELM_DOCS_VERSION)
HELM_DOCS_REPO := github.com/norwoodj/helm-docs/cmd/helm-docs

$(HELM_DOCS): ## Installs helm-docs
	@echo === installing helm-docs
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOBIN=$(TOOLS_HOST_DIR)/tmp GO111MODULE=on go install $(HELM_DOCS_REPO)@$(HELM_DOCS_VERSION)
	@mv $(TOOLS_HOST_DIR)/tmp/helm-docs $(HELM_DOCS)
	@rm -fr $(TOOLS_HOST_DIR)/tmp
