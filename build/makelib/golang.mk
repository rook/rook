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

GO_SUPPORTED_VERSIONS ?= 1.18|1.19

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
GOLINT := $(TOOLS_HOST_DIR)/golint
GOJUNIT := $(TOOLS_DIR)/go-junit-report

GO := go
GOHOST := GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go
GO_VERSION := $(shell $(GO) version | sed -ne 's/[^0-9]*\(\([0-9]\.\)\{0,4\}[0-9][^.]\).*/\1/p')
GO_FULL_VERSION := $(shell $(GO) version)

GOFMT_VERSION := $(GO_VERSION)
ifneq ($(findstring $(GOFMT_VERSION),$(GO_VERSION)),)
GOFMT := $(shell which gofmt)
else
GOFMT := $(TOOLS_HOST_DIR)/gofmt$(GOFMT_VERSION)
endif

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
	$(error unsupported: $(GO_FULL_VERSION). Please make install one of the following supported version: '$(GO_SUPPORTED_VERSIONS)')
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
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -cover $(GO_STATIC_FLAGS) $(GO_PACKAGES)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -cover $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_PACKAGES) 2>&1 | tee $(GO_TEST_OUTPUT)/unit-tests.log
	@cat $(GO_TEST_OUTPUT)/unit-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/unit-tests.xml

.PHONY:
go.test.integration: $(GOJUNIT)
	@echo === go test integration-tests
	@mkdir -p $(GO_TEST_OUTPUT)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -i $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES)
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) test -v -timeout 7200s $(GO_TEST_FLAGS) $(GO_STATIC_FLAGS) $(GO_INTEGRATION_TEST_PACKAGES) $(TEST_FILTER_PARAM) 2>&1 | tee $(GO_TEST_OUTPUT)/integration-tests.log
	@cat $(GO_TEST_OUTPUT)/integration-tests.log | $(GOJUNIT) -set-exit-code > $(GO_TEST_OUTPUT)/integration-tests.xml

.PHONY: go.lint
go.lint: $(GOLINT)
	@echo === go lint
	@$(GOLINT) -set_exit_status=true $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.vet
go.vet:
	@echo === go vet
	CGO_ENABLED=$(CGO_ENABLED_VALUE) $(GOHOST) vet $(GO_COMMON_FLAGS) $(GO_PACKAGES) $(GO_INTEGRATION_TEST_PACKAGES)

.PHONY: go.fmt
# ignore deepcopy generated files since the tool hardcoded the header with a "// +build" which in Golang 1.17 makes it fail gofmt since "////go:build" is preferred
# see: https://github.com/kubernetes/gengo/blob/master/examples/deepcopy-gen/generators/deepcopy.go#L136
# https://github.com/kubernetes/gengo/pull/210
go.fmt: $(GOFMT)
	@gofmt_out=$$(find $(GO_SUBDIRS) -name "*.go" -not -name "*.deepcopy.go" | xargs $(GOFMT) -s -d -e 2>&1) && [ -z "$${gofmt_out}" ] || (echo "$${gofmt_out}" 1>&2; exit 1)

go.validate: go.vet go.fmt

.PHONY: go.mod.update
go.mod.update:
	@echo === updating modules
	@$(GOHOST) get -u ./...

.PHONY: go.mod.check
go.mod.check:
	@echo === ensuring modules are tidied
	@$(GOHOST) mod tidy -compat=1.17

.PHONY: go.mod.clean
go.mod.clean:
	@echo === cleaning modules cache
	@sudo rm -fr $(WORK_DIR)/cross_pkg
	@$(GOHOST) clean -modcache

$(GOLINT):
	@echo === installing golint
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@GOPATH=$(TOOLS_HOST_DIR)/tmp GOBIN=$(TOOLS_HOST_DIR) $(GOHOST) get github.com/golang/lint/golint
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOFMT):
	@echo === installing gofmt$(GOFMT_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
	@curl -sL https://dl.google.com/go/go$(GOFMT_VERSION).$(shell go env GOHOSTOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp
	@mv $(TOOLS_HOST_DIR)/tmp/go/bin/gofmt $(GOFMT)
	@rm -fr $(TOOLS_HOST_DIR)/tmp

$(GOJUNIT):
	@echo === installing go-junit-report
	@mkdir -p $(TOOLS_DIR)/tmp
	@curl -sL https://github.com/jstemmer/go-junit-report/releases/download/v2.0.0/go-junit-report-v2.0.0-$(GOOS)-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_DIR)/tmp
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

YQ_VERSION = v4.14.2
YQ := $(TOOLS_HOST_DIR)/yq-$(YQ_VERSION)
export YQ
$(YQ):
	@echo === installing yq $(YQ_VERSION) $(REAL_HOST_PLATFORM)
	@mkdir -p $(TOOLS_HOST_DIR)
	@curl -JL https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(REAL_HOST_PLATFORM) -o $(YQ)
	@chmod +x $(YQ)

export CODE_GENERATOR_VERSION=0.20.0
export CODE_GENERATOR=$(TOOLS_HOST_DIR)/code-generator-$(CODE_GENERATOR_VERSION)
$(CODE_GENERATOR):
	mkdir -p $(TOOLS_HOST_DIR)
	curl -sL https://github.com/kubernetes/code-generator/archive/refs/tags/v${CODE_GENERATOR_VERSION}.tar.gz | tar -xz -C $(TOOLS_HOST_DIR)
