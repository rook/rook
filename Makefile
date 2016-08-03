REPOPATH = github.com/quantum/castle
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)

# TODO: support for building outside of a $GOPATH
# TODO: support for profiling in ceph
# TODO: support for tracing in ceph
# TODO: stripping binaries (at least the c binaries since golang does not like to be stripped)
# TODO: jemalloc and static linking are currently broken due to https://github.com/jemalloc/jemalloc/issues/442

# Can be used for additional go build flags
BUILDFLAGS ?=
LDFLAGS ?=

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

# set to 1 for a completely static build
STATIC ?= 0
ifeq ($(STATIC),1)
LDFLAGS += -extldflags "-static"
BUILDFLAGS += -installsuffix cgo
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

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v
BUILDFLAGS += -x
endif

# set the version number. 
ifeq ($(origin VERSION), undefined)
VERSION = $(shell git describe --dirty --always)
endif
LDFLAGS += -X $(REPOPATH)/pkg/version.Version=$(VERSION)

.PHONY: all
all: build test

.PHONY: build
build: vendor ceph
	mkdir -p ceph/build
	cd ceph/build && cmake $(CEPHD_CMAKE) ..
	cd ceph/build && $(MAKE) $(MAKEFLAGS) cephd
	strip -S ceph/build/lib/libcephd.a
	go build $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o bin/castled ./cmd/castled

ceph:
	git submodule update --init --recursive

.PHONY: release
release: build

.PHONY: test
test: tools/glide
	go test --race $(shell ./tools/glide novendor)

.PHONY: vet
vet: tools/glide
	go vet $(shell ./tools/glide novendor)

.PHONY: fmt
fmt: tools/glide
	go fmt $(shell ./tools/glide novendor)

.PHONY: clean
clean:
	rm -rf bin
	rm -rf ceph/build

.PHONY: cleanall
cleanall: clean
	rm -rf tools vendor

vendor: tools/glide
	./tools/glide install

tools/glide:
	@echo "Downloading glide"
	mkdir -p tools
	curl -L https://github.com/Masterminds/glide/releases/download/v0.11.1/glide-v0.11.1-$(GOOS)-$(GOARCH).tar.gz | tar -xz -C tools
	mv tools/$(GOOS)-$(GOARCH)/glide tools/glide
	rm -r tools/$(GOOS)-$(GOARCH)
