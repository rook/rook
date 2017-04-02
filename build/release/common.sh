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

    shift 3
    local bins="$@"

    local bindir=$(get_bindir ${os} ${arch})

    mkdir -p $dir/root/usr/bin
    mkdir -p $dir/root/etc/ssl/certs

    for b in $bins; do
        cp $bindir/$b $dir/root/usr/bin
    done

    cp /etc/ssl/certs/ca-certificates.crt $dir/root/etc/ssl/certs
}

check_release_version() {
    if echo ${RELEASE_VERSION} | grep -q -E '^([[:digit:]]+)\.([[:digit:]]+)\.([[:digit:]]+)$'; then
        return 0
    else
        return 1
    fi
}

github_get_release_id() {
    curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
       "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/tags/${RELEASE_VERSION}" | jq -r '.id'
}

github_create_release() {
    id=$(github_get_release_id)

    if [[ ${id} == "null" ]]; then
        curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{\"tag_name\": \"${RELEASE_VERSION}\",\"target_commitish\": \"master\",\"name\": \"${RELEASE_VERSION}\",\"body\": \"TBD\",\"draft\": true,\"prerelease\": true}" \
            "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases"
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

s3_upload() {
    local filepath=$1
    local filename=$(basename $filepath)

    # we assume that AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and possibly AWS_DEFAULT_REGION are already set
    # or ~/.aws/credentials and ~/.aws/config are configured
    aws s3 cp --only-show-errors ${filepath} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/${RELEASE_VERSION}/${filename}
}

s3_promote_file() {
    local filename=$1

    aws s3 cp --only-show-errors s3://${RELEASE_S3_BUCKET}/master/${RELEASE_VERSION}/${filename} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/${RELEASE_VERSION}/${filename}
    aws s3 cp --only-show-errors s3://${RELEASE_S3_BUCKET}/master/${RELEASE_VERSION}/${filename} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/current/${filename}
}
