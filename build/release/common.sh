#!/bin/bash -e

get_bindir() {
    local os=$1
    local arch=$2

    bindir=${RELEASE_BIN_DIR}/${os}_${arch}
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
    cp $bindir/castle $dir/root/usr/bin

    cp /etc/ssl/certs/ca-certificates.crt $dir/root/etc/ssl/certs
}

github_get_release_id() {
    curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
       "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/tags/${RELEASE_VERSION}" | jq -r '.id'
}

github_check_release() {
    id=$(github_get_release_id)

    if [[ ${id} == "null" ]]; then
        echo "ERROR: github release tag ${RELEASE_VERSION} was not found. Did you create the release?" 1>&2
        return 1;
    fi
}

github_upload() {
    local filepath=$1
    local mediatype=$2
    local filename=$(basename $filepath)

    id=$(github_get_release_id)

    curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
         -H "Accept: application/vnd.github.manifold-preview" \
         -H "Content-Type: application/${mediatype}" \
         --data-binary @${filepath} \
         "https://uploads.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/${id}/assets?name=${filename}"
}
