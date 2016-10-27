.PHONY: all build publish
all: build

VERSION := $(shell cat version)
CONTAINER := quay.io/rook/cross-build:$(VERSION)

build: Dockerfile entrypoint.sh
	@docker build ${BUILD_ARGS} -t $(CONTAINER) --pull $(CURDIR)

publish: build
	docker push $(CONTAINER)
