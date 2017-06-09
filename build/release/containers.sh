#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

build_registry=${BUILD_REGISTRY}
registry=${RELEASE_REGISTRY}

get_build_image_name() {
    local os=$1
    local arch=$2
    local repo=$3

    local image=${repo}-${arch}:latest

    echo ${build_registry}/${image}
}

get_image_name() {
    local os=$1
    local arch=$2
    local repo=$3
    local version=$4

    local image=${repo}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        image=${repo}:${version}
    fi

    echo ${registry}/${image}
}

build_image() {
    local os=$1
    local arch=$2
    local repo=$3

    local build_image=$(get_build_image_name $os $arch $repo)
    local image=$(get_image_name $os $arch $repo ${RELEASE_VERSION})
    docker tag ${build_image} ${image}
}

build() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    build_image $os $arch rookd
}

publish_image() {
    local os=$1
    local arch=$2
    local repo=$3

    local name=$(get_image_name $os $arch $repo ${RELEASE_VERSION})

    echo pushing docker container ${name}
    docker push ${name}

    # we will always tag master builds as latest. i.e. auto-promote master
    if [[ "${RELEASE_CHANNEL}" == "master" ]]; then
        local dst=$(get_image_name $os $arch $repo master-latest)
        echo pushing docker container ${dst}
        docker tag ${name} ${dst}
        docker push ${dst}
    fi
}

publish() {
    local os=$1
    local arch=$2

    [[ ${os} == "linux" ]] || return 0

    publish_image $os $arch rookd
# disabled for now
#    publish_image $os $arch rook
#    [[ ${arch} == "amd64" ]] || return 0
#    publish_image $os $arch toolbox
}

promote_image() {
    local os=$1
    local arch=$2
    local repo=$3

    local src=$(get_image_name $os $arch $repo ${RELEASE_VERSION})
    local dst1=$(get_image_name $os $arch $repo ${RELEASE_CHANNEL}-latest)
    local dst2=$(get_image_name $os $arch $repo ${RELEASE_CHANNEL}-${RELEASE_VERSION})

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

    promote_image $os $arch rookd
#    promote_image $os $arch rook
#    promote_image $os $arch toolbox
}

cleanup_image() {
    local os=$1
    local arch=$2
    local repo=$3
    local img

    for t in \
        ${RELEASE_VERSION} \
        ${RELEASE_CHANNEL}-latest \
        ${RELEASE_CHANNEL}-${RELEASE_VERSION} \
        ; do
        img=$(get_image_name $os $arch $repo ${t})
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
    cleanup_image $os $arch rookd
#    cleanup_image $os $arch rook
#    cleanup_image $os $arch toolbox
}

action=$1
shift

${action} "$@"
