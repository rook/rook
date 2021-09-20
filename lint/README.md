# Rook project linting

In this directory are all Rook linter configurations which are used for github.com/rook CI linting.

You can run the "Super Linter" locally.
```shell
# assuming $PWD is the path to the rook repo (e.g., ~/go/src/github.com/rook/rook)
docker run -e RUN_LOCAL=true -e LINTER_RULES_PATH=lint -v $PWD:/tmp/lint \
           -e VALIDATE_YAML=true \
           -e VALIDATE_GO=true -e CGO_ENABLED=0 \
           -e VALIDATE_BASH=true -e SHELLCHECK_OPTS="--external-sources --severity=warning" \
    github/super-linter:slim-v4
```
