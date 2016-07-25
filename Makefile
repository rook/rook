ifeq ($(origin VERSION), undefined)
	VERSION != git describe --dirty --always
endif

REPOPATH = github.com/quantum/castle

GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

NPROCS:=1
ifeq ($(GOOS),linux)
  NPROCS:=$(shell grep -c ^processor /proc/cpuinfo)
endif

build: vendor ceph/build/lib/libcephd.a
	@for i in castled; do \
		go build -o bin/$$i -installsuffix cgo -ldflags "-extldflags '-static' -linkmode external -X $(REPOPATH)/version.Version=$(VERSION)" ./cmd/$$i; \
	done

ceph/build/Makefile:
	git submodule update --init --recursive
	cd ceph && ./do_cmake.sh -DWITH_EMBEDDED=ON -DWITH_FUSE=OFF

ceph/build/lib/libcephd.a: ceph/build/Makefile
	cd ceph/build && make -j$(NPROCS) cephd

test: tools/glide
	go test --race $(shell ./tools/glide novendor)

vet: tools/glide
	go vet $(shell ./tools/glide novendor)

fmt: tools/glide
	go fmt $(shell ./tools/glide novendor)

clean:
	rm -rf ./bin
	cd ceph/build && make clean

cleanall: clean
	rm -rf bin tools vendor

vendor: tools/glide
	./tools/glide install

tools/glide:
	@echo "Downloading glide"
	mkdir -p tools
	curl -L https://github.com/Masterminds/glide/releases/download/v0.11.1/glide-v0.11.1-$(GOOS)-$(GOARCH).tar.gz | tar -xz -C tools
	mv tools/$(GOOS)-$(GOARCH)/glide tools/glide
	rm -r tools/$(GOOS)-$(GOARCH)
