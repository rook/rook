.PHONY: all
all: build

# ====================================================================================
# Build Options

# Can be used for additional go build flags
BUILDFLAGS ?=
LDFLAGS ?=
TAGS ?=

# set to 1 for a completely static build. Otherwise if set to 0
# a dynamic binary is produced that requires glibc to be installed
STATIC ?= 1
ifeq ($(STATIC),1)
CASTLED_BUILDFLAGS += -installsuffix cgo
CASTLED_LDFLAGS += -extldflags "-static"
CASTLED_TAGS += static
else
CASTLED_TAGS += dynamic
endif

# build a position independent executable. This implies dynamic linking
# since statically-linked PIE is not supported by the linker/glibc
PIE ?= 0
ifeq ($(PIE),1)
ifeq ($(STATIC),1)
$(error PIE only supported with dynamic linking. Set STATIC=0.)
endif
CASTLED_BUILDFLAGS += -buildmode=pie
CASTLED_TAGS += pie
endif

# if DEBUG is set to 1 debug information is perserved (i.e. not stripped). 
# the binary size is going to be much larger.
DEBUG ?= 0

ALLOCATOR ?= tcmalloc_minimal
CCACHE ?= 1

# turn on more verbose build
V ?= 0
ifeq ($(V),1)
LDFLAGS += -v
BUILDFLAGS += -x
else
MAKEFLAGS += --no-print-directory
endif

# set the version number. 
ifeq ($(origin VERSION), undefined)
VERSION = $(shell git describe --dirty --always)
endif
LDFLAGS += -X $(REPO)/pkg/version.Version=$(VERSION)

# ====================================================================================
# Setup Go projects

GO_ORG=github.com/quantum
GO_PROJ=castle
include build/golang.mk

# add castlectl and castled. this will define all the necessary
# rules to build the projects and also set targets such as build, clean, etc.
$(eval $(call golang-project,castlectl,cmd/castlectl, \
	$(BUILDFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)'))
$(eval $(call golang-project,castled,cmd/castled, \
	$(BUILDFLAGS) $(CASTLED_BUILDFLAGS) -tags '$(TAGS) $(CASTLED_TAGS)' -ldflags '$(LDFLAGS) $(CASTLED_LDFLAGS)'))

# ====================================================================================
# Setup Cephd

CEPHD_DEBUG = $(DEBUG)
CEPHD_CCACHE = $(CCACHE)
CEPHD_ALLOCATOR = $(ALLOCATOR)
include build/cephd.mk

# ====================================================================================
# Targets

.PHONY: build test check vet fmt clean buildall cleanall

build: go.build.castlectl

check test: go.test.castlectl

vet: go.vet.castlectl go.vet.castled

fmt: go.fmt.castlectl go.fmt.castled

clean: go.clean.castlectl  go.clean.castled

cleanall: go.cleanall cephd.cleanall

buildall: \
	go.build.castlectl.linux_amd64 \
	go.build.castlectl.linux_arm64 \
	go.build.castlectl.darwin_amd64 \
	go.build.castlectl.windows_amd64

ifeq ($(GO_HOST_OS),linux)
ifeq ($(GO_HOST_ARCH),amd64)

$(eval $(call cephd-build,linux_amd64))

go.test.castled bin/linux_amd64/castled: $(CEPHD_BUILD_DIR)/linux_amd64/lib/libcephd.a

clean: cephd.clean.linux_amd64

build: bin/linux_amd64/castled

check test: go.test.castled

buildall: bin/linux_amd64/castled

else
ifeq ($(GO_HOST_ARCH),arm64)

$(eval $(call cephd-build,linux_arm64))

go.test.castled bin/linux_arm64/castled: $(CEPHD_BUILD_DIR)/linux_arm64/lib/libcephd.a

clean: cephd.clean.linux_arm64

build: bin/linux_arm64/castled

check test: go.test.castled

buildall: bin/linux_arm64/castled

endif # arm64
endif # amd64
endif # linux

# ====================================================================================
# Help

.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Targets:'
	@echo '    build              Build project.'
	@echo '    buildall           Build project for all platforms.'
	@echo '    check              Builds and runs tests on.'
	@echo '    clean              Remove binaries.'
	@echo '    cleanall           Remove binaries for all platforms.'
	@echo '    fmt                Check formatting of go sources.'
	@echo '    help               Show this help info.'
	@echo '    vendor             Installs vendor dependencies.'
	@echo '    vet                Runs lint checks on go sources.'
	@echo ''
	@echo 'Options:'
	@echo ''
	@echo '    CCACHE             Set to 1 to enabled ccache, 0 to disable.' 
	@echo '                       The default is 0.'
	@echo '    DEBUG              Set to 1 to disable stripping the binaries of.'
	@echo '                       debug information. The default is 0.'
	@echo '    PIE                Set to 1 to build build a position independent'
	@echo '                       executable. Can not be combined with static.'
	@echo '                       The default is 0.'
	@echo '    STATIC             Set to 1 for static build, 0 for dynamic.'
	@echo '                       The default is 1.'
	@echo '    VERSION            The version information compiled into binaries.'
	@echo '                       The default is obtained from git.'
	@echo '    V                  Set to 1 enable verbose build. Default is 0.'
