# ====================================================================================
# Makefile helper funcions for golang

# TODO: check golang version
# TODO: can we strip -s all binaries to reduce size?

# ====================================================================================
# Configuration

ifeq ($(GO_ORG),)
$(error the variable $$GO_ORG must be set prior to including golang.mk)
endif

ifeq ($(GO_PROJ),)
$(error the variable $$GO_PROJ must be set prior to including golang.mk)
endif

GO_SOURCE_DIRS ?= ./cmd ./pkg

GO_BIN_DIR ?= bin
GO_WORK_DIR ?= .gowork
GO_TOOLS_DIR := tools
GO_VENDOR_DIR := vendor

GO_OS ?= $(shell go env GOOS)
GO_ARCH ?= $(shell go env GOARCH)
GO_HOST_OS = $(shell go env GOHOSTOS)
GO_HOST_ARCH = $(shell go env GOHOSTARCH)

# TODO: check version
GO_MIN_VERSION ?= 1.6

# ====================================================================================
# Internal variables

GO_SOURCES := $(shell find $(GO_SOURCE_DIRS) -name '*.go' | sed 's/ /\\ /g')

GO_PATH ?= $(GO_WORK_DIR)/.gopath
GO_REPO= $(GO_ORG)/$(GO_PROJ)
GO_ORG_PATH = $(GO_PATH)/src/$(GO_ORG)
GO_REPO_PATH = $(GO_ORG_PATH)/$(GO_PROJ)
GO_VERSION := $(shell go version 2>&1 | awk -Fgo '{ print $3 }' | awk '{ print $1 }')

export GOPATH=$(CURDIR)/$(GO_PATH)
export GOOS=$(GO_OS)
export GOARCH=$(GO_ARCH)

# ====================================================================================
# Targets

$(GO_WORK_DIR):
	@mkdir -p $(GO_WORK_DIR)

$(GO_REPO_PATH): | $(GO_WORK_DIR) 
	@mkdir -p $(GO_ORG_PATH)
	@ln -s ../../../../.. $(GO_REPO_PATH)

$(GO_TOOLS_DIR):
	@mkdir -p $(GO_TOOLS_DIR)

# generate a list of dependent packages and sources
# $1 target name (for example castlectl)
# $2 the root package of the project (for example cmd/castlectl)
define golang-deps
$(GO_WORK_DIR)/$(1).gosrcs.mk: $(GO_SOURCES) | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@go list -f '{{.ImportPath}}{{printf "\n"}}{{range $$$$d := .Deps}}{{$$$$d}}{{printf "\n"}}{{end}}' $(2) | grep $(GO_REPO) | grep --invert-match vendor > $$@.tmp
	@echo "$(1).gopkgs := \\" > $$@
	@cat $$@.tmp | tr '\n' ' ' >> $$@
	@echo "" >> $$@
	@echo "$(1).gosrcs := \\" >> $$@
	@go list -f '{{range $$$$f := .GoFiles}}{{$$$$.Dir}}/{{$$$$f}} {{end}}\' $$$$(cat $$@.tmp) >>$$@
	@go list -f '{{range $$$$f := .CgoFiles}}{{$$$$.Dir}}/{{$$$$f}} {{end}}\' $$$$(cat $$@.tmp) >>$$@
	@go list -f '{{range $$$$f := .TestGoFiles}}{{$$$$.Dir}}/{{$$$$f}} {{end}}\' $$$$(cat $$@.tmp) >>$$@
	@echo "" >> $$@
	@rm $$@.tmp

-include $(GO_WORK_DIR)/$(1).gosrcs.mk
endef

# generate targets to build a go project
# $1 target name (for example castlectl)
# $2 the root package of the project (for example github.com/quantum/castle/cmd/castlectl)
# $3 optional args that will be passed to go build
# $4 support platforms
define golang-build
.PRECIOUS: $(GO_BIN_DIR)/%/$(1)
$(GO_BIN_DIR)/%/$(1): $$($(1).gosrcs) $(GO_WORK_DIR)/vendor | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Building $(1) for $$*"
	@GOOS=$$(word 1, $$(subst _, ,$$*)) GOARCH=$$(word 2, $$(subst _, ,$$*)) go build $(3) -o $$@ $(2)

.PHONY: go.build.$(1)
go.build.$(1): $(GO_BIN_DIR)/$(GO_HOST_OS)_$(GO_HOST_ARCH)/$(1) ;
endef

# generate targets to test a go project
# $1 target name (for example castlectl)
# $2 optional args that will be passed to go test
define golang-test
.PHONY: go.test.$(1)
go.test.$(1): $$($(1).gosrcs) $(GO_WORK_DIR)/vendor | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Running go test on target $(1)"
	@GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go test $(2) $$($(1).gopkgs)
endef

# generate targets to vet a go project
# $1 target name (for example castlectl)
# $2 optional args that will be passed to go vet
define golang-vet
$(GO_WORK_DIR)/$(1).vet: $$($(1).gosrcs) | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Running go vet on target $(1)"
	@GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go vet $(2) $$($(1).gopkgs)
	@touch $$@
.PHONY: go.vet.$(1)
go.vet.$(1): $(GO_WORK_DIR)/$(1).vet
endef

# generate targets to fmt a go project
# $1 target name (for example castlectl)
# $2 optional args that will be passed to go fmt
define golang-fmt
$(GO_WORK_DIR)/$(1).fmt: $$($(1).gosrcs) | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Running go fmt on target $(1)"
	@GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go fmt $(2) $$($(1).gopkgs)
	@touch $$@
.PHONY: go.fmt.$(1)
go.fmt.$(1): $(GO_WORK_DIR)/$(1).fmt
endef

# generate targets to clean a go project
# $1 target name (for example castlectl)
# $2 optional args that will be passed to go clean
define golang-clean
.PHONY: go.clean.$(1)
go.clean.$(1): | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Running go clean on target $(1)"
	@GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go clean $(2) $$($(1).gopkgs)
endef

# generate targets for a go project
# $1 target name (for example castlectl)
# $2 the root package of the project (for example cmd/castlectl)
# $3 optional args that will be passed to go tools
define golang-project
$(call golang-deps,$1,$(GO_REPO)/$2)
$(call golang-build,$1,$(GO_REPO)/$2,$3)
$(call golang-vet,$1,$3)
$(call golang-fmt,$1)
$(call golang-test,$1,$3)
$(call golang-clean,$1,$3)
endef

.PHONY: vendor
vendor $(GO_WORK_DIR)/vendor: $(GO_TOOLS_DIR)/glide | $(GO_WORK_DIR) $(GO_REPO_PATH)
	@echo "====== Installing vendor packages"
	@$(GO_TOOLS_DIR)/glide install
	@touch $@

$(GO_TOOLS_DIR)/glide: | $(GO_TOOLS_DIR)
	@echo "====== Installing glide"
	@curl -sL https://github.com/Masterminds/glide/releases/download/v0.12.2/glide-v0.12.2-$(GO_HOST_OS)-$(GO_HOST_ARCH).tar.gz | tar -xz -C $(GO_TOOLS_DIR)
	@mv $(GO_TOOLS_DIR)/$(GO_HOST_OS)-$(GO_HOST_ARCH)/glide $(GO_TOOLS_DIR)/glide
	@rm -r $(GO_TOOLS_DIR)/$(GO_HOST_OS)-$(GO_HOST_ARCH)

go.cleanall:
	@rm -rf $(GO_TOOLS_DIR) $(GO_VENDOR_DIR) $(GO_BIN_DIR) $(GO_REPO_PATH) $(GO_WORK_DIR)

cleanall: go.cleanall

# go supports cross compiling
ifneq ($(GOOS),linux)
ifeq ($(GOARCH),amd64)
CC=x86_64-linux-gnu-gcc
CXX=x86_64-linux-gnu-g++
STRIP=x86_64-linux-gnu-strip
endif
ifeq ($(GOARCH),arm64)
CC=aarch64-linux-gnu-gcc
CXX=aarch64-linux-gnu-g++
STRIP=aarch64-linux-gnu-strip
endif
endif # linux
