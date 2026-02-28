# Expands to list this project's go packages, excluding the vendor folder
SHELL = bash

all: fmt build test lint

build:
	go build
	

lint:
	golint -set_exit_status ./...

clean:
	rm -rf build

fmt:
	go fmt ./...

test:
	go test ./...

testall:
	go test -count 1 ./...

coverage:
	@if [ ! -d build ]; then mkdir build; fi
	# runs go test and generate coverage report
	go test -covermode=count -coverprofile=build/coverage.out ./...
	go tool cover -html=build/coverage.out -o build/coverage.html

bench:
	go test -bench ./...

### TOOLS

tools:
	go install golang.org/x/tools/cmd/cover@latest
	go install golang.org/x/lint/golint@latest

.PHONY: all build lint clean fmt test coverage tools

