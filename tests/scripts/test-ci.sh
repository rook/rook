#!/usr/bin/env bash
set -xeuo pipefail

function push() {
    arm64_Master_Image=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "arm64:latest" | head -n 1)
    docker tag "$arm64_Master_Image" quay.io/rook/rook-arm64:master
    docker push quay.io/rook/rook-arm64:master
    docker tag "$arm64_Master_Image" ghcr.io/rook/rook-arm64:master
    docker push ghcr.io/rook/rook-arm64:master

    arm64_Release_Image=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep -v "arm64:latest" | grep -v "arm64:master" | grep arm64 | head -n 1)
    tag_arm64_Release_Image="$(echo "$arm64_Release_Image" | cut -d ":" -f 2)"
    docker tag "$arm64_Release_Image" quay.io/rook/rook-arm64:"$tag_arm64_Release_Image"
    docker push quay.io/rook/rook-arm64:"$tag_arm64_Release_Image"
    docker tag "$arm64_Release_Image" ghcr.io/rook/rook-arm64:"$tag_arm64_Release_Image"
    docker push ghcr.io/rook/rook-arm64:"$tag_arm64_Release_Image"

    amd64_Master_Image=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "amd64:latest" | head -n 1)
    docker tag "$amd64_Master_Image" quay.io/rook/rook-amd64:master
    docker push quay.io/rook/rook-amd64:master
    docker tag "$amd64_Master_Image" ghcr.io/rook/rook-amd64:master
    docker push ghcr.io/rook/rook-amd64:master

    amd64_Release_Image=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "amd64" | grep -v "amd64:latest" | grep -v "amd64:master" | head -n 1)
    tag_amd64_Release_Image="$(echo "$amd64_Release_Image" | cut -d ":" -f 2)"
    docker tag "$amd64_Release_Image" quay.io/rook/rook-amd64:"$tag_amd64_Release_Image"
    docker push quay.io/rook/rook-amd64:"$tag_amd64_Release_Image"
    docker tag "$amd64_Release_Image" ghcr.io/rook/rook-amd64:"$tag_amd64_Release_Image"
    docker push ghcr.io/rook/rook-amd64:"$tag_amd64_Release_Image"

    curl -o manifest-tool -LO https://github.com/estesp/manifest-tool/releases/download/v1.0.2/manifest-tool-linux-amd64
    chmod +x manifest-tool

    # Create and push multi-arch manifests
    ./manifest-tool push from-args --platforms linux/amd64,linux/arm64 --template quay.io/rook/rook-ARCH:master --target quay.io/rook/ceph:master
    ./manifest-tool push from-args --platforms linux/amd64,linux/arm64 --template ghcr.io/rook/rook-ARCH:master --target ghcr.io/rook/ceph:master

    ./manifest-tool push from-args --platforms linux/amd64,linux/arm64 --template quay.io/rook/rook-ARCH:$tag_amd64_Release_Image --target quay.io/rook/ceph:$tag_amd64_Release_Image
    ./manifest-tool push from-args --platforms linux/amd64,linux/arm64 --template ghcr.io/rook/rook-ARCH:$tag_amd64_Release_Image --target ghcr.io/rook/ceph:$tag_amd64_Release_Image
}

push
