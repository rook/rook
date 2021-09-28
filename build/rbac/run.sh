#!/usr/bin/env bash

set -eEuo pipefail

docker build . --tag rbac

this_dir="$PWD"

docker run --rm -it \
    -v $PWD:/go/src/github.com/rook/rook \
    -v
