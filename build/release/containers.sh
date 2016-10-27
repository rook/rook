#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

registry=quay.io/
repo=rook/rookd

build() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    tmpdir=$(mktemp -d)
    trap "rm -fr $tmpdir" EXIT

    layout_root $os $arch $tmpdir
    mkdir $tmpdir/root/tmp

    cat <<EOF > $tmpdir/Dockerfile
FROM alpine:3.4
RUN apk add --no-cache gptfdisk util-linux kmod coreutils grep gawk e2fsprogs btrfs-progs sudo
COPY root /
ENTRYPOINT ["/usr/bin/rookd"]
EOF

    tag=${repo}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        tag=${repo}:${version}
    fi

    echo building docker container ${tag}
    docker build -t ${registry}${tag} $tmpdir

    local file=${tag/\//-}
    local file=${file/:/-}
    local dockerout=${RELEASE_DIR}/${file}.docker
    echo ${file}

    echo generate ACIs from docker containers
    docker save -o ${dockerout} ${registry}${tag}
    docker2aci ${dockerout}
    mv *.aci ${RELEASE_DIR}

    rm -fr $tmpdir
}

publish() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    tag=${repo}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        tag=${repo}:${version}
    fi

    echo pushing docker container ${tag}
    docker push ${registry}${tag}
}

action=$1
shift

${action} "$@"
