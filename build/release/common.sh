#!/bin/bash -e

ghargs="-u ${GITHUB_USER} -r ${GITHUB_REPO} -t ${RELEASE_VERSION}"

get_bindir() {
    local os=$1
    local arch=$2

    bindir=${RELEASE_BIN_DIR}
    if [[ ${os}_${arch} != ${RELEASE_HOST_PLATFORM} ]]; then
        bindir=${bindir}/${os}_${arch}
    fi

    echo ${bindir}
}

layout_root() {
    local os=$1
    local arch=$2
    local dir=$3
    local bindir=$(get_bindir ${os} ${arch})

    mkdir -p $dir/root/usr/bin
    mkdir -p $dir/root/etc/ssl/certs

    cp $bindir/castled $dir/root/usr/bin
    cp $bindir/castlectl $dir/root/usr/bin

    cp /etc/ssl/certs/ca-certificates.crt $dir/root/etc/ssl/certs
}

github_check_release() {
    if ! github-release info ${ghargs} > /dev/null 2>&1; then
        echo "ERROR: github release tag ${version} was not found. Did you create the release?" 1>&2
        return 1;
    fi
}

github_upload() {
    local file=$1
    github-release upload ${ghargs} -n $(basename $file) -f $file
}

