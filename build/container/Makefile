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

.PHONY: all build publish
all: build

VERSION := $(shell cat version)
CONTAINER := quay.io/rook/cross-build:$(VERSION)

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
BUILD_ARGS=--build-arg VERBOSE=1
endif

build: Dockerfile entrypoint.sh
	docker build -t $(CONTAINER) ${BUILD_ARGS} $(CURDIR)

rebuild: Dockerfile entrypoint.sh
	docker build --no-cache --pull -t $(CONTAINER) ${BUILD_ARGS} $(CURDIR)

publish: build
	docker push $(CONTAINER)
