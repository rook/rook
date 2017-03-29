#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

registry=quay.io/

build() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    tmpdir=$(mktemp -d)
    trap "rm -fr $tmpdir" EXIT
    cat <<EOF > $tmpdir/Dockerfile
FROM alpine:3.5
RUN apk add --no-cache gptfdisk util-linux coreutils
COPY root /
ENTRYPOINT ["/usr/bin/rookd"]
EOF
    build_artifact $os $arch $tmpdir rook/rookd rookd rook

    tmpdir=$(mktemp -d)
    trap "rm -fr $tmpdir" EXIT
    cat <<EOF > $tmpdir/Dockerfile
FROM alpine:3.5
COPY root /
ENTRYPOINT ["/usr/bin/rook-operator"]
EOF
    build_artifact $os $arch $tmpdir rook/rook-operator rook-operator
}

build_artifact() {
    local os=$1
    local arch=$2
    local tmpdir=$3
    local repo=$4
    local version=${RELEASE_VERSION}

    shift 4
    local bins="$@"

    layout_root $os $arch $tmpdir $bins
    mkdir $tmpdir/root/tmp

    tag=${repo}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        tag=${repo}:${version}
    fi

    echo building docker container ${tag}
    docker build -t ${registry}${tag} $tmpdir

    local file=${tag/\//-}
    local file=${file/:/-}
    local dockerout=${file}.docker
    echo ${file}

    echo generate ACIs from docker containers
    (cd ${RELEASE_DIR} && docker save -o ${dockerout} ${registry}${tag})
    (cd ${RELEASE_DIR} && docker2aci ${dockerout})

    rm -fr $tmpdir
}

publish() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    publish_artifact $os $arch rook/rookd
    publish_artifact $os $arch rook/rook-operator
}

publish_artifact() {
    local os=$1
    local arch=$2
    local repo=$3
    local version=${RELEASE_VERSION}

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
