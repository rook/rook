#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

get_archive_ext() {
    local os=$1

    local ext=tar.gz
    if [[ ${os} != "linux" ]]; then
        ext=zip
    fi

    echo ${ext}
}

get_archive_name() {
    local os=$1
    local arch=$2
    local suffix=$3

    local file=rook-${os}-${arch}

    if [[ -n ${suffix} ]]; then
        file=${file}-${suffix}
    fi

    local ext=$(get_archive_ext $os)

    echo ${file}.${ext}
}

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

        local tarfile=$(get_archive_name $os $arch)
        echo creating tar ${tarfile}
        tar czf "${RELEASE_DIR}/${tarfile}" -C "${bindir}" ${files[*]}
    else
        local zipfile=$(get_archive_name $os $arch)
        local zippath=$(realpath ${RELEASE_DIR}/${zipfile})
        echo creating zip ${zipfile}
        $(cd ${bindir} && zip -qr ${zippath} ${files[*]})
    fi
}

publish() {
    local os=$1
    local arch=$2
    local file=$(get_archive_name $os $arch)

    s3_upload ${RELEASE_DIR}/$file

    if [[ ${os} == "linux" ]]; then
        file=$(get_archive_name $os $arch "debug")
        s3_upload ${RELEASE_DIR}/$file
    fi

    # we will always tag master builds as latest. i.e. auto-promote master
    if [[ "${RELEASE_CHANNEL}" == "master" ]]; then
        s3_promote_release
    fi
}

promote() {
    local os=$1
    local arch=$2
    local file=$(get_archive_name $os $arch)

    if [[ ! -e ${RELEASE_DIR}/$file ]]; then
        s3_download ${RELEASE_DIR}/$file
    fi
    github_upload ${RELEASE_DIR}/$file

    if [[ ${os} == "linux" ]]; then
        file=$(get_archive_name $os $arch "debug")
        if [[ ! -e ${RELEASE_DIR}/$file ]]; then
            s3_download ${RELEASE_DIR}/$file
        fi
        github_upload ${RELEASE_DIR}/$file
    fi
}

cleanup() {
    :
}

action=$1
shift

${action} "$@"
