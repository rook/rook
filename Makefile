ORG = github.com/quantum
PROJ = castle
GOPATH = $(CURDIR)/.gopath
ORGPATH = $(GOPATH)/src/$(ORG)
REPOPATH = $(ORGPATH)/$(PROJ)
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)

export GOPATH

# TODO: support for profiling in ceph
# TODO: support for tracing in ceph
# TODO: jemalloc and static linking are currently broken due to https://github.com/jemalloc/jemalloc/issues/442
# TODO: remove leveldb from ceph
# TODO: cross compile castlectl?
# TODO: can we strip -s all binaries?

# Can be used for additional go build flags
BUILDFLAGS ?=
LDFLAGS ?=
TAGS ?= 

# Additional flags to use when calling cmake on ceph
CEPHD_CMAKE += \
	-DWITH_EMBEDDED=ON \
	-DWITH_FUSE=OFF \
	-DWITH_NSS=OFF \
	-DUSE_CRYPTOPP=ON \
	-DWITH_LEVELDB=OFF \
	-DWITH_XFS=OFF \
	-DWITH_OPENLDAP=OFF \
	-DWITH_MANPAGE=OFF \
	-DWITH_PROFILER=OFF

# set to 1 for a completely static build. Otherwise if set to 0
# a dynamic binary is produced that requires glibc to be installed
STATIC ?= 1
ifeq ($(STATIC),1)
BUILDFLAGS += -installsuffix cgo
LDFLAGS += -extldflags "-static"
TAGS += static
else
TAGS += dynamic
endif

# build a position independent executable. This implies dynamic linking
# since statically-linked PIE is not supported by the linker/glibc
PIE ?= 0
ifeq ($(PIE),1)
ifeq ($(STATIC),1)
$(error PIE only supported with dynamic linking. Set STATIC=0.)
endif
BUILDFLAGS += -buildmode=pie
TAGS += pie
endif

# Can be jemalloc, tcmalloc or glibc 
ALLOCATOR ?= tcmalloc
ifeq ($(ALLOCATOR),jemalloc)
CEPHD_CMAKE += -DALLOCATOR=jemalloc
TAGS += jemalloc
else 
ifeq ($(ALLOCATOR),tcmalloc)
CEPHD_CMAKE += -DALLOCATOR=tcmalloc
TAGS += tcmalloc
else
CEPHD_CMAKE += -DALLOCATOR=libc
endif
endif

# additional go build tags
TAGS ?= 

# if DEBUG is set to 1 debug information is perserved (i.e. not stripped). 
# the binary size is going to be much larger.
DEBUG ?= 0

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v
BUILDFLAGS += -x
else
MAKEFLAGS += --no-print-directory
endif

# set to 0 to disable ccache support
# must also export CCACHE_DIR to take effect
CCACHE ?= 1
ifeq ($(CCACHE),1)
CEPHD_CMAKE += -DWITH_CCACHE=ON
endif

# set the version number. 
ifeq ($(origin VERSION), undefined)
VERSION = $(shell git describe --dirty --always)
endif
LDFLAGS += -X $(ORG)/$(PROJ)/pkg/version.Version=$(VERSION)

GO_SOURCEDIRS=cmd pkg
GO_SOURCES := $(shell find $(GO_SOURCEDIRS) -name '*.go')
GO_PKGS=$(shell go list ./... | grep --invert-match vendor)

.PHONY: all
all: build

.PHONY: build
build: bin/castled bin/castlectl vet fmt

.PHONY: release
release: build

.PHONY: check
check: test

$(REPOPATH):
	@mkdir -p $(ORGPATH)
	@ln -s ../../../.. $(REPOPATH)

bin/castled: $(REPOPATH) vendor/vendor.stamp ceph/build $(GO_SOURCES) 
	@cd ceph/build && $(MAKE) cephd
	@if test ceph/build/lib/libcephd.a -nt bin/castled -o ! -f bin/castled; then \
		if test $(DEBUG) -eq 0; then \
			strip -S ceph/build/lib/libcephd.a; \
		fi; \
		go build $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o bin/castled ./cmd/castled; \
	fi

bin/castlectl: $(REPOPATH) vendor/vendor.stamp $(GO_SOURCES) 
	@go build $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o bin/castlectl ./cmd/castlectl

# we take a dependency on this Makefile since the ceph cmake config is in it 
ceph/build: Makefile
	@mkdir -p ceph/build
	@cd ceph/build && cmake $(CEPHD_CMAKE) ..

.PHONY: vendor
vendor: tools/glide
	@./tools/glide install

# the stamp file ensures that we run glide install once. every other time is manual
vendor/vendor.stamp: tools/glide
	@./tools/glide install
	@touch $@

.PHONY: test
test: build
	@go test -cover -timeout 30s $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' $(GO_PKGS)

.PHONY: vet
vet: $(REPOPATH) tools/glide $(GO_SOURCES)
	@go vet $(shell ./tools/glide novendor)

.PHONY: fmt
fmt: $(REPOPATH) tools/glide $(GO_SOURCES)
	@go fmt $(shell ./tools/glide novendor)

.PHONY: cleancommon
cleancommon:
	@rm -rf bin
	@rm -fr $(GOPATH)
	@if [ -d ceph/src/rocksdb ]; then cd ceph/src/rocksdb && $(MAKE) clean > /dev/null; fi

.PHONY: clean
clean: cleancommon
	@if [ -d ceph/build ]; then cd ceph/build && $(MAKE) clean; fi

.PHONY: cleanall
cleanall: cleancommon
	@rm -rf tools vendor
	@rm -rf ceph/build

tools/glide:
	@mkdir -p tools
	@curl -sL https://github.com/Masterminds/glide/releases/download/v0.12.2/glide-v0.12.2-$(GOOS)-$(GOARCH).tar.gz | tar -xz -C tools
	@mv tools/$(GOOS)-$(GOARCH)/glide tools/glide
	@rm -r tools/$(GOOS)-$(GOARCH)
