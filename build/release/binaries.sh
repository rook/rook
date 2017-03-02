#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

build() {
    local os=$1
    local arch=$2
    local version=${RELEASE_VERSION}
    local bindir=$(get_bindir ${os} ${arch})

    if [[ ${os} == "windows" ]]; then
        local ext=".exe"
    fi

    local files=( rook${ext} )

    if [[ ${os} == "linux" ]]; then
        files+=( rookd${ext} )

        local tarfile=${RELEASE_DIR}/rook-${version}-${os}-${arch}.tar.gz
        echo creating tar ${tarfile}
        tar czf "${tarfile}" -C "${bindir}" ${files[*]}

        # create a package with debug symbols
        files=( rookd${ext}.debug )
        local tarfile=${RELEASE_DIR}/rook-${version}-${os}-${arch}-debug.tar.gz
        echo creating debug tar ${tarfile}
        tar czf "${tarfile}" -C "${bindir}" ${files[*]}
    else
        local zipfile=$(realpath ${RELEASE_DIR}/rook-${version}-${os}-${arch}.zip)
        echo creating zip ${zipfile}
        $(cd ${bindir} && zip -qr ${zipfile} ${files[*]})
    fi
}

publish() {
    local os=$1
    local arch=$2
    local version=${RELEASE_VERSION}

    local file=${RELEASE_DIR}/rook-${version}-${os}-${arch}
    local ext=tar.gz
    local mediatype=gzip

    if [[ ${os} != "linux" ]]; then
        ext=zip
        mediatype=gzip
    fi

    echo uploading $file.$ext to github
    github_upload $file.$ext $mediatype

    if [[ ${os} == "linux" ]]; then
        echo uploading $file-debug.$ext to github
        github_upload $file-debug.$ext $mediatype
    fi
}

action=$1
shift

${action} "$@"
