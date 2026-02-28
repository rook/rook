SHELL = bash
BUILD_FLAGS =
TEST_FLAGS =

all: fmt tidy build lint test

build:
	go build $(BUILD_FLAGS) ./...

builddir:
	mkdir -p -m 0777 build

lint:
	golangci-lint run

clean:
	rm -rf build/*

fmt:
	go fmt ./...

test:
	go test -race $(BUILD_FLAGS) $(TEST_FLAGS) ./...

# creates a test coverage report, and produces json test output.  useful for ci.
cover: builddir
	go test $(TEST_FLAGS) -v -covermode=count -coverprofile=build/coverage.out -json ./...
	go tool cover -html=build/coverage.out -o build/coverage.html

builder:
	docker-compose build --pull builder

docker: builder
	docker-compose run --rm builder make all cover

fish: builder
	docker-compose run --rm builder fish

tidy:
	go mod tidy

update:
	go get -u ./...
	go mod tidy

### TOOLS

tools:
# installs tools used during build
	go get -u golang.org/x/tools/cmd/cover
	sh -c "$$(wget -O - -q https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh || echo exit 2)" -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: all build builddir run artifacts vet lint clean fmt test testall testreport up down pull builder runc ci bash fish image prep vendor.update vendor.ensure tools buildtools migratetool db.migrate

