#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

build() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}
    local bindir=$(get_bindir ${os} ${arch})

    if [[ ${os} == "windows" ]]; then
        local ext=".exe"
    fi

    local files=( castle${ext} )

    if [[ ${type} == "both" ]]; then
        files+=( castled${ext} )
    fi

    mkdir -p ${RELEASE_DIR}

    if [[ ${os} == "linux" ]]; then
        local tarfile=${RELEASE_DIR}/castle-${version}-${os}-${arch}.tar.gz
        echo creating tar ${tarfile}
        tar czf "${tarfile}" -C "${bindir}" ${files[*]}
    else
        local zipfile=$(realpath ${RELEASE_DIR}/castle-${version}-${os}-${arch}.zip)
        echo creating zip ${zipfile}
        $(cd ${bindir} && zip -qr ${zipfile} ${files[*]})
    fi
}

publish() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    local file=${RELEASE_DIR}/castle-${version}-${os}-${arch}
    local ext=tar.gz
    local mediatype=gzip

    if [[ ${os} != "linux" ]]; then
        ext=zip
        mediatype=gzip
    fi

    echo uploading $file.$ext to github
    github_upload $file.$ext $mediatype
}

action=$1
shift

${action} "$@"
