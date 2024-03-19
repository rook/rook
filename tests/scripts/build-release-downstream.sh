#!/usr/bin/env bash
set -ex

# Load dot env file if available
if [ -f .env ]; then
    # shellcheck disable=SC2046
    export $(grep -v '^#' .env | xargs -d '\n')
fi

MAKE='make --debug=v --output-sync'
$MAKE build BUILD_REGISTRY=local
build_Image="local/ceph-amd64:latest"
git_hash=$(git rev-parse --short "${GITHUB_SHA}")
tag_Image=quay.io/ocs-dev/rook-ceph:v${BRANCH_NAME}-$git_hash
docker tag "$build_Image" "$tag_Image"
docker push "$tag_Image"
