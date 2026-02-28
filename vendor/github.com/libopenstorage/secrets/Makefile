generate:
	go generate ./...

unit-test:
	go test ./...

ci-test:
	go test -timeout 1800s -v github.com/libopenstorage/secrets/vault -tags ci

integration-test:
	go test -timeout 1800s -v github.com/libopenstorage/secrets/vault -tags integration
