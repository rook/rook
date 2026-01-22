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

# ====================================================================================
# Makefile helper functions for golang
#

ifeq ($(GO_PROJECT),)
$(error the variable GO_PROJECT must be set prior to including golang.mk)
endif

# These targets will be statically linked.
GO_STATIC_PACKAGES ?=

ifeq ($(GO_STATIC_PACKAGES),)
$(error please set GO_STATIC_PACKAGES prior to including golang.mk)
endif

# These are the static test packages
GO_STATIC_PACKAGES ?=

# Optional. These are subdirs that we look for all go files to test, vet, and fmt
GO_SUBDIRS ?= cmd pkg

# Optional. Additional subdirs used for integration or e2e testings
GO_INTEGRATION_TESTS_SUBDIRS ?= tests

# Optional directories (relative to CURDIR)
GO_PKG_DIR ?= $(WORK_DIR)/pkg

# Optional build flags passed to go tools
GO_BUILDFLAGS ?=
GO_LDFLAGS ?=
GO_TAGS ?=
GO_TEST_FLAGS ?=

# ====================================================================================
# Setup go environment

GO_SUPPORTED_VERSIONS ?= 1.25

GO_PACKAGES := $(foreach t,$(GO_SUBDIRS),$(GO_PROJECT)/$(t)/...)
GO_INTEGRATION_TEST_PACKAGES := $(foreach t,$(GO_INTEGRATION_TESTS_SUBDIRS),$(GO_PROJECT)/$(t)/integration)

ifneq ($(GO_TEST_SUITE),)
GO_TEST_FLAGS += -run '$(GO_TEST_SUITE)'
endif

ifneq ($(GO_TEST_FILTER),)
TEST_FILTER_PARAM := -testify.m '$(GO_TEST_FILTER)'
endif

GOPATH := $(shell go env GOPATH)

# setup tools used during the build
GOJUNIT := $(TOOLS_DIR)/go-junit-report

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go
GO_VERSION := $(shell $(GO) version | sed -ne 's/[^0-9]*\(\([0-9]\.\)\{0,4\}[0-9][^.]\).*/\1/p')
GO_FULL_VERSION := $(shell $(GO) version)

YQ_VERSION = v4.45.1
YQ := $(TOOLS_HOST_DIR)/yq-$(YQ_VERSION)
export YQ
$(YQ):
	@echo === installing yq $(YQ_VERSION) $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(REAL_HOST_PLATFORM) -o $(YQ)
	@chmod +x $(YQ)

GOLANGCI_LINT_VERSION := $(strip $(shell $(YQ) .jobs.golangci.steps[2].with.version .github/workflows/golangci-lint.yaml))
GOLANGCI_LINT := $(TOOLS_HOST_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)

# KAL is a modified version of golangci-lint with itself built in as an addon. Downloading KAL as a
# separate binary allows caching the binary between 'make' calls (vs building it into golangci-lint)
# KAL version list: https://pkg.go.dev/sigs.k8s.io/kube-api-linter?tab=versions
export KUBE_API_LINT_VERSION=v0.0.0-20260105171240-d42ba1d7b50c
export KUBE_API_LINT=$(TOOLS_HOST_DIR)/golangci-lint-kube-api-linter-$(KUBE_API_LINT_VERSION)

GO_OUT_DIR := $(abspath $(OUTPUT_DIR)/bin/$(PLATFORM))
GO_TEST_OUTPUT := $(abspath $(OUTPUT_DIR)/tests/$(PLATFORM))

ifeq ($(GOOS),windows)
GO_OUT_EXT := .exe
endif

# NOTE: the install suffixes are matched with the build container to speed up the
# the build. Please keep them in sync.

ifneq ($(GO_PKG_DIR),)
GO_PKG_BASE_DIR := $(abspath $(GO_PKG_DIR)/$(PLATFORM))
GO_PKG_STATIC_FLAGS := -pkgdir $(GO_PKG_BASE_DIR)_static
endif

GO_COMMON_FLAGS = $(GO_BUILDFLAGS) -tags '$(GO_TAGS)'
GO_STATIC_FLAGS = $(GO_COMMON_FLAGS) $(GO_PKG_STATIC_FLAGS) -installsuffix static  -ldflags '$(GO_LDFLAGS)'

# ====================================================================================
# Targets

ifeq ($(filter help clean distclean prune go.clean, $(MAKECMDGOALS)),)
.PHONY: go.check
go.check:
ifneq ($(shell $(GO) version | grep -q -E '\bgo($(GO_SUPPORTED_VERSIONS))\b' && echo 0 || echo 1), 0)
	$(error unsupported: $(GO_FULL_VERSION). Please install one of the following supported versions: '$(GO_SUPPORTED_VERSIONS)')
endif

-include go.check
endif

.PHONY: go.init
go.init:
	@:

.PHONY: go.build
go.build:
	@echo === go build $(PLATFORM)
	$(info Go version: $(shell $(GO) version))
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GO) build -v -o $(GO_OUT_DIR)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_STATIC_FLAGS) $(p)${\n})
	$(foreach p,$(GO_TEST_PACKAGES),@CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GO) test -v -c -o $(GO_TEST_OUTPUT)/$(lastword $(subst /, ,$(p)))$(GO_OUT_EXT) $(GO_STATIC_FLAGS) $(p)${\n})

.PHONY: go.install
go.install:
	@echo === go install $(PLATFORM)
	$(foreach p,$(GO_STATIC_PACKAGES),@CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GO) install -v $(GO_STATIC_FLAGS) $(p)${\n})

# GOJUNIT need to happen in order and NOT in parallel, so call them explicitly
.PHONY: go.test.unit
go.test.unit:
	@$(MAKE) $(GOJUNIT)
	@echo === go test unit-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GO) test -v -cover $(GO_STATIC_FLAGS) $(GO_PACKAGES)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GO) test -v -cover $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_PACKAGES) 2>&1 | tee $(GO_TEST_OUTPUT)/unit-tests.log
	@cat $(GO_TEST_OUTPUT)/unit-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/unit-tests.xml

.PHONY:
go.test.integration: $(GOJUNIT)
	@echo === go test integration-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -i $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -timeout 7200s $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES) $(TEST_FILTER_PARAM) 2>&1 | tee $(GO_TEST_OUTPUT)/integration-tests.log
	@cat $(GO_TEST_OUTPUT)/integration-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/integration-tests.xml

.PHONY: go.vet
go.vet:
	@echo === go vet
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) vet $(GO_COMMON_FLAGS) $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.fmt
go.fmt: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) fmt -d

.PHONY: go.fmt-fix
go.fmt-fix: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) fmt

.PHONY: go.golangci-lint
go.golangci-lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run

.PHONY: go.kube-api-lint
# When KAL was added, 1310 errors were reported in Rook's APIs. Show all by default.
KUBE_API_LINT_OPTIONS ?= --max-issues-per-linter=2000
go.kube-api-lint: $(KUBE_API_LINT)
	cd $(ROOT_DIR)/pkg/apis && $(KUBE_API_LINT) run --config $(ROOT_DIR)/tests/scripts/kube-api-lint.yaml $(KUBE_API_LINT_OPTIONS)

go.validate: go.vet go.fmt

.PHONY: go.mod.update
go.mod.update:
	@echo === updating modules
	@$(GOHOST) get -u ./...

.PHONY: go.mod.check
go.mod.check:
	@echo === syncing root modules with APIs modules
	@cp -a go.sum pkg/apis/go.sum
	@cat go.mod | sed -e 's|^module github.com/rook/rook|module github.com/rook/rook/pkg/apis|' \
	                  -e '\:^replace github.com/rook/rook/pkg/apis => ./pkg/apis:d' > pkg/apis/go.mod
	@echo === ensuring APIs modules are tidied
	@(cd pkg/apis/; $(GOHOST) mod tidy -compat=$(GO_VERSION))
	@echo === ensuring root modules are tidied
	@$(GOHOST) mod tidy -compat=$(GO_VERSION)

.PHONY: go.mod.clean
go.mod.clean:
	@echo === cleaning modules cache
	@sudo rm -fr $(WORK_DIR)/cross_pkg
	@$(GOHOST) clean -modcache

$(GOLANGCI_LINT):
	@echo === installing golangci-lint-$(GOLANGCI_LINT_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/golangci-lint-$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))-$(shell go env GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/golangci-lint-$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))-$(shell go env GOHOSTOS)-$(GOHOSTARCH)/golangci-lint $(GOLANGCI_LINT)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(KUBE_API_LINT):
	@echo === installing kube-api-lint@$(KUBE_API_LINT_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)
	GOBIN=$(TOOLS_HOST_DIR) go install sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter@$(KUBE_API_LINT_VERSION)
	@ mv $(TOOLS_HOST_DIR)/golangci-lint-kube-api-linter $(KUBE_API_LINT)

$(GOJUNIT):
	@echo === installing go-junit-report
	@mkdir -p $(TOOLS_DIR)/tmp
	@curl -sL https://github.com/jstemmer/go-junit-report/releases/download/v2.1.0/go-junit-report-v2.1.0-$(GOOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_DIR)/tmp
	@mv $(TOOLS_DIR)/tmp/go-junit-report $(TOOLS_DIR)
	@rm -fr $(TOOLS_DIR)/tmp

export CONTROLLER_GEN=$(TOOLS_HOST_DIR)/controller-gen-$(CONTROLLER_GEN_VERSION)
$(CONTROLLER_GEN):
	{ \
		set -e ;\
		mkdir -p $(TOOLS_HOST_DIR) ;\
		CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
		cd $$CONTROLLER_GEN_TMP_DIR ;\
		go mod init tmp;\
		unset GOOS GOARCH ;\
		export CGO_ENABLED=$(CGO_ENABLED_VALUE) ;\
		export GOBIN=$$CONTROLLER_GEN_TMP_DIR ;\
		echo === installing controller-gen ;\
		go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION);\
		mv $$CONTROLLER_GEN_TMP_DIR/controller-gen $(CONTROLLER_GEN) ;\
		rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}


export CODE_GENERATOR_VERSION=0.34.2
export CODE_GENERATOR=$(TOOLS_HOST_DIR)/code-generator-$(CODE_GENERATOR_VERSION)
$(CODE_GENERATOR):
	mkdir -p $(TOOLS_HOST_DIR)
	curl -sL https://github.com/kubernetes/code-generator/archive/refs/tags/v${CODE_GENERATOR_VERSION}.tar.gz | tar -xz -C $(TOOLS_HOST_DIR)
