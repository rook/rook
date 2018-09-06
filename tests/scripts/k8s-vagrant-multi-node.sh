#!/usr/bin/env bash

set -ex

WORK_DIR="/tmp/k8s-vagrant-multi-node"

download() {
    if [ ! -d "${WORK_DIR}" ]; then
        git clone https://github.com/galexrt/k8s-vagrant-multi-node.git "${WORK_DIR}"
    fi
    cd "${WORK_DIR}" || { echo "Unable to access k8s-vagrant-multi-node project at ${WORK_DIR}"; exit 1; }
    git fetch origin
    TAG="$(git describe --tags "$(git rev-list --tags --max-count=1)")"
    echo "Checking out taq ${TAG}"
    git checkout "${TAG}"
}

remove() {
    rm -rf "${WORK_DIR}"
}

copy_images() {
    if [[ "$1" == "" || "$1" == "ceph" ]]; then
        echo "copying ceph images"
        make -j"$(nproc)" load-image IMG="${BUILD_REGISTRY}/ceph-amd64" TAG="rook/ceph:master"
        make -j"$(nproc)" load-image IMG="${BUILD_REGISTRY}/ceph-toolbox-amd64" TAG="rook/ceph-toolbox:master"
    fi

    if [[ "$1" == "" || "$1" == "cockroachdb" ]]; then
        echo "copying cockroachdb image"
        make -j"$(nproc)" load-image IMG="${BUILD_REGISTRY}/cockroachdb-amd64" TAG="rook/cockroachdb:master"
    fi

    if [[ "$1" == "" || "$1" == "minio" ]]; then
        echo "copying minio image"
        make -j"$(nproc)" load-image IMG="${BUILD_REGISTRY}/minio-amd64" TAG="rook/minio:master"
    fi

    if [[ "$1" == "" || "$1" == "nfs" ]]; then
        echo "copying nfs image"
        make -j"$(nproc)" load-image IMG="${BUILD_REGISTRY}/nfs-amd64" TAG="rook/nfs:master"
    fi
}

download
cd "${WORK_DIR}" || { echo "Unable to access k8s-vagrant-multi-node project at ${WORK_DIR}"; exit 1; }

case "${1}" in
    up)
        make -j"$(nproc)" up
        ;;
    down|stop)
        make -j"$(nproc)" stop
        ;;
    clean)
        make -j"$(nproc)" clean
        cd || { echo "Couldn't cd to 'default' directory."; exit 1; }
        remove
        ;;
    ssh-*)
        make "${1}"
        ;;
    update)
        copy_images "$2"
        ;;
    *)
        echo "usage:" >&2
        echo "  ${0} up" >&2
        echo "  ${0} down" >&2
        echo "  ${0} ssh-(master|node-[1-X])" >&2
        echo "  ${0} update" >&2
        echo >&2
        echo "If you want more control over the multi node environment," >&2
        echo "go to ${WORK_DIR} and use 'make' directly." >&2
        ;;
esac

exit 0
