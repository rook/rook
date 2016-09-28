.PHONY: all buildall publishall
all: buildall

buildall: build-amd64 build-arm64
build-%: Dockerfile.in entrypoint.sh
	$(eval ARCH := $*)
	$(eval TMPDIR := $(shell mktemp -d))
	@echo building cross-build container for $(ARCH)
	@sed 's/==ARCH==/$(ARCH)/g' Dockerfile.in > $(TMPDIR)/Dockerfile
	@if [ $(ARCH) = "amd64" ]; then \
		sed -i 's/==BUILD==/build-essential/g' $(TMPDIR)/Dockerfile; \
	else \
		sed -i 's/==BUILD==/crossbuild-essential-$(ARCH)/g' $(TMPDIR)/Dockerfile; \
	fi
	@cp entrypoint.sh $(TMPDIR)/
	@cd $(TMPDIR) && docker build -t quantum/castle-build-$* .
	@rm -fr $(TMPDIR)

publishall: publish-amd64 publish-arm64
publish-%: build-%
	docker push quantum/castle-build-$*
