#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

docker save -o rookamd64.tar  ${BUILD_REGISTRY}/rook-amd64
docker save -o toolboxamd64.tar ${BUILD_REGISTRY}/toolbox-amd64
