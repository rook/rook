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

get_mediatype_from_extension() {
    local file=$1
    local ext=${file##*.}
    local mediatype

    case ${ext} in
        gz) echo "application/gzip" ;;
        zip) echo "application/zip" ;;
        *) echo "UNSUPPORTED" ;;
    esac
}

github_get_release_id() {
    curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
       "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/tags/${RELEASE_VERSION}" | jq -r '.id'
}

github_get_upload_url() {
    id=`cat ${RELEASE_DIR}/release_id`
    curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
       "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/${id}" | jq -r '.upload_url'
}

github_create_release() {
    id=$(github_get_release_id)

    if [[ ${id} == "null" ]]; then
        echo creating a new github release for ${RELEASE_VERSION}
        id=$(curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{\"tag_name\": \"${RELEASE_VERSION}\",\"target_commitish\": \"master\",\"name\": \"${RELEASE_VERSION}\",\"body\": \"TBD\",\"draft\": true,\"prerelease\": true}" \
            "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases" | jq -r '.id')

        if [[ ${id} == "null" ]]; then
            echo error: failed to create github release
            return 1
        fi

        cat <<EOF > ${RELEASE_DIR}/release_id
${id}
EOF
    fi
}

github_delete_release() {
    id=$(github_get_release_id)

    if [[ ${id} != "null" ]]; then
        echo deleting existing github release ${id} for ${RELEASE_VERSION}
        curl -4 -s -X DELETE -H "Authorization: token ${GITHUB_TOKEN}" \
                "https://api.github.com/repos/${GITHUB_USER}/${GITHUB_REPO}/releases/${id}"
    fi
}

github_create_or_replace_release() {
    github_delete_release
    github_create_release
}

github_upload() {
    local filepath=$1
    local mediatype=$(get_mediatype_from_extension $filepath)
    local filename=$(basename $filepath)

    upload_url=$(github_get_upload_url)

    if [[ ${upload_url} == "null" ]]; then
        echo error: could not get upload url for github release
        return 1
    fi

    if [[ ! -e ${filepath} ]]; then
        echo error: could not find file ${filepath}
        return 1
    fi

    upload_url=${upload_url%{*} # remove the trailing {?name,label}

    echo uploading ${filename} github release ${RELEASE_VERSION}
    id=$(curl -4 -s -H "Authorization: token ${GITHUB_TOKEN}" \
         -H "Accept: application/vnd.github.manifold-preview" \
         -H "Content-Type: ${mediatype}" \
         --data-binary @${filepath} \
         "${upload_url}?name=${filename}" | jq -r '.id')
    if [[ ${id} == "null" ]]; then
        echo error: failed to upload ${filename} to github
        return 1
    fi
}

github_release_complete() {
    rm -fr ${RELEASE_DIR}/release_id
}

write_version_file() {
    # upload a file with the version number
    cat <<EOF > ${RELEASE_DIR}/version
${version}
EOF
}

publish_version_file() {
    s3_upload ${RELEASE_DIR}/version

    if [[ "${RELEASE_CHANNEL}" == "master" ]]; then
        s3_promote_file version
    fi
}

# we assume that AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and possibly AWS_DEFAULT_REGION are already set
# or ~/.aws/credentials and ~/.aws/config are configured

s3_upload() {
    local filepath=$1
    local filename=$(basename $filepath)

    echo uploading ${filename} to S3 bucket ${RELEASE_S3_BUCKET}
    aws s3 cp --only-show-errors ${filepath} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/${RELEASE_VERSION}/${filename}
}

s3_download() {
    local filepath=$1
    local filename=$(basename $filepath)

    echo downloading ${filename} from S3 bucket ${RELEASE_S3_BUCKET}
    aws s3 cp --only-show-errors s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/${RELEASE_VERSION}/${filename} ${filepath}
}

s3_promote_file() {
    local filename=$1

    echo copying ${filename} from master to ${RELEASE_CHANNEL} in S3 bucket ${RELEASE_S3_BUCKET}
    if [[ "${RELEASE_CHANNEL}" != "master" ]]; then
        aws s3 cp --only-show-errors s3://${RELEASE_S3_BUCKET}/master/${RELEASE_VERSION}/${filename} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/${RELEASE_VERSION}/${filename}
    fi
}

s3_promote_release() {
    echo copying ${RELEASE_VERSION} to current for channel ${RELEASE_CHANNEL} in S3 bucket ${RELEASE_S3_BUCKET}
    aws s3 sync --only-show-errors --delete s3://${RELEASE_S3_BUCKET}/master/${RELEASE_VERSION} s3://${RELEASE_S3_BUCKET}/${RELEASE_CHANNEL}/current
}
