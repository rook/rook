#Copyright 2018 The Rook Authors. All rights reserved.
#
#Licensed under the Apache License, Version 2.0 (the "License");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an "AS IS" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.


include ../image.mk

# ====================================================================================
# Image Build Options

CASSANDRA_OPERATOR_IMAGE = $(BUILD_REGISTRY)/cassandra-$(GOARCH)

TEMP := $(shell mktemp -d)

# ====================================================================================
# Build Rook Cassandra

do.build:
	@echo === docker build $(CASSANDRA_OPERATOR_IMAGE)
	@cp Dockerfile $(TEMP)
	@cp $(OUTPUT_DIR)/bin/linux_$(GOARCH)/rook $(TEMP)
	@$(DOCKERCMD) build $(BUILD_ARGS) \
		--build-arg ARCH=$(GOARCH) \
    	--build-arg TINI_VERSION=$(TINI_VERSION) \
		-t $(CASSANDRA_OPERATOR_IMAGE) \
		$(TEMP)
	@rm -fr $(TEMP)
