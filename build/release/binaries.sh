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

    if [[ ${os} != "linux" ]]; then
        ext=zip
    fi

    echo uploading $file.$ext to S3
    s3_upload $file.$ext

    if [[ ${os} == "linux" ]]; then
        echo uploading $file-debug.$ext to S3
        s3_upload $file-debug.$ext
    fi

    # upload a file with the version number
    tmpdir=$(mktemp -d)
    cat <<EOF > $tmpdir/version
${version}
EOF
    s3_upload $tmpdir/version
}

cleanup() {
    :
}

action=$1
shift

${action} "$@"
