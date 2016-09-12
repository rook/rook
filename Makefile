REPOPATH = github.com/quantum/castle
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)

# TODO: support for building outside of a $GOPATH
# TODO: support for profiling in ceph
# TODO: support for tracing in ceph
# TODO: jemalloc and static linking are currently broken due to https://github.com/jemalloc/jemalloc/issues/442
# TODO: remove leveldb
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
MFLAGS += -w
else
MFLAGS += -s
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
	@mkdir -p ceph/build
	@echo "##### configuring ceph" 
	cd ceph/build && cmake $(CEPHD_CMAKE) ..
	@echo "##### building ceph" 
	cd ceph/build && $(MAKE) $(MFLAGS) cephd
ifeq ($(DEBUG),0)
	@echo "##### stripping libcephd.a" 
	strip -S ceph/build/lib/libcephd.a
endif
	@echo "##### building castled" 
	go build $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o bin/castled ./cmd/castled
	@echo "##### building castlectl" 
	go build $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o bin/castlectl ./cmd/castlectl

ceph:
	git submodule update --init --recursive

.PHONY: release
release: build

.PHONY: test
test: tools/glide
	go test -cover -timeout 30s $(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' $(shell ./tools/glide novendor)

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
