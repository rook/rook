#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

registry=${RELEASE_REGISTRY}/

get_image_name() {
    local os=$1
    local arch=$2
    local repo=$3
    local version=$4

    local tag=${repo}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        tag=${repo}:${version}
    fi

    echo ${tag}
}

build() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    make -C ${scriptdir}/../../images cross PLATFORMS=${arch} VERSION=${RELEASE_VERSION}
}

publish_artifact() {
    local os=$1
    local arch=$2
    local repo=$3

    local name=${registry}$(get_image_name $os $arch $repo ${RELEASE_VERSION})

    echo pushing docker container ${name}
    docker push ${name}

    # we will always tag master builds as latest. i.e. auto-promote master
    if [[ "${RELEASE_CHANNEL}" == "master" ]]; then
        local dst=${registry}$(get_image_name $os $arch $repo master-latest)
        echo pushing docker container ${dst}
        docker tag ${name} ${dst}
        docker push ${dst}
    fi
}

publish() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    publish_artifact $os $arch rook/rookd 
    publish_artifact $os $arch rook/rook

    # TODO: publish the toolbox for arm
    [[ ${arch} == "amd64" ]] || return 0
    publish_artifact $os $arch rook/toolbox
}

promote_artifact() {
    local os=$1
    local arch=$2
    local repo=$3

    local src=${registry}$(get_image_name $os $arch $repo ${RELEASE_VERSION})
    local dst1=${registry}$(get_image_name $os $arch $repo ${RELEASE_CHANNEL}-latest)
    local dst2=${registry}$(get_image_name $os $arch $repo ${RELEASE_CHANNEL}-${RELEASE_VERSION})

    echo promoting container ${src} to ${dst1} and ${dst2}
    docker pull ${src}
    docker tag ${src} ${dst1}
    docker tag ${src} ${dst2}
    docker push ${dst1}
    docker push ${dst2}
}

promote() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    promote_artifact $os $arch rook/rookd
    promote_artifact $os $arch rook/rook
    promote_artifact $os $arch rook/toolbox
}

cleanup_artifact() {
    local os=$1
    local arch=$2
    local repo=$3
    local img

    for t in \
        ${RELEASE_VERSION} \
        ${RELEASE_CHANNEL}-latest \
        ${RELEASE_CHANNEL}-${RELEASE_VERSION} \
        ; do
        img=${registry}$(get_image_name $os $arch $repo ${t})
        if [[ -n "$(docker images -q ${img} 2> /dev/null)" ]]; then
            echo removing docker image ${img}
            docker rmi ${img} || true
        fi
    done
}

cleanup() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0
    cleanup_artifact $os $arch rook/rookd
    cleanup_artifact $os $arch rook/rook
    cleanup_artifact $os $arch rook/toolbox
}

action=$1
shift

${action} "$@"
