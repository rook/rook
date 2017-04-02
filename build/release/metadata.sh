#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

build() {
    # upload a file with the version number
    cat <<EOF > ${RELEASE_DIR}/version
${version}
EOF
}

publish() {
    s3_upload ${RELEASE_DIR}/version
}

promote() {
    s3_promote_file version
}

cleanup() {
    :
}

action=$1
shift

${action} "$@"
