# Copyright 2026 The Rook Authors. All rights reserved.
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
# Makefile helper functions for the documentation site, built with Hugo: https://gohugo.io/
#
# The Hugo project is in .docs/; its content is Documentation/. Hugo needs `go` to download the
# theme, which is a Hugo module (.docs/go.mod).

HUGO_VERSION := v0.168.0
HUGO := $(TOOLS_HOST_DIR)/hugo-$(HUGO_VERSION)

DOCS_HUGO_DIR := $(ROOT_DIR)/.docs
# where docs-build writes the site
DOCS_SITE_DIR ?= $(ROOT_DIR)/site
# the URL the site is published at
DOCS_BASE_URL ?= /
# links to files on the master branch of the rook repo are changed to point to this branch
DOCS_GITHUB_BRANCH ?= $(shell git -C $(ROOT_DIR) rev-parse --abbrev-ref HEAD)

DOCS_HUGO := HUGO_PARAMS_GITHUBBRANCH=$(DOCS_GITHUB_BRANCH) $(HUGO) --source $(DOCS_HUGO_DIR)

$(HUGO): | $(TOOLS_HOST_DIR) ## Installs hugo
	@echo === installing hugo
	@rm -rf $(TOOLS_HOST_DIR)/tmp
	@mkdir -p $(TOOLS_HOST_DIR)/tmp
ifeq ($(shell go env GOHOSTOS),darwin)
	@# for macOS hugo is released as an installer package only; extract the binary from it
	@curl -fsSL -o $(TOOLS_HOST_DIR)/tmp/hugo.pkg https://github.com/gohugoio/hugo/releases/download/$(HUGO_VERSION)/hugo_extended_$(HUGO_VERSION:v%=%)_darwin-universal.pkg
	@pkgutil --expand-full $(TOOLS_HOST_DIR)/tmp/hugo.pkg $(TOOLS_HOST_DIR)/tmp/pkg
	@mv $(TOOLS_HOST_DIR)/tmp/pkg/Payload/hugo $(HUGO)
else
	@curl -fsSL https://github.com/gohugoio/hugo/releases/download/$(HUGO_VERSION)/hugo_extended_$(HUGO_VERSION:v%=%)_linux-$(GOHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp hugo
	@mv $(TOOLS_HOST_DIR)/tmp/hugo $(HUGO)
endif
	@rm -rf $(TOOLS_HOST_DIR)/tmp
