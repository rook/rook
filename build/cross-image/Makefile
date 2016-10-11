.PHONY: all build publish
all: build

VERSION := $(shell cat version)
CONTAINER := quantum/castle-cross-build:$(VERSION)

build: Dockerfile entrypoint.sh
	@docker build ${BUILD_ARGS} -t $(CONTAINER) --pull $(CURDIR)

publish: build
	docker push $(CONTAINER)
