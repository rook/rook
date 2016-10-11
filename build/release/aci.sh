#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

container=quantum.com/castle

build() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    tmpdir=$(mktemp -d)
    trap "rm -fr $tmpdir" EXIT

    layout_root $os $arch $tmpdir

    mkdir -p ${RELEASE_DIR}

    local acb="acbuild --debug --work-path $tmpdir"
    local acifile=${RELEASE_DIR}/castle-${version}-${os}-${arch}.aci

    echo creating aci file ${acifile}

    ${acb} begin
    ${acb} set-name ${container}
    ${acb} copy-to-dir $tmpdir/root /
    ${acb} label add version "$version"
    ${acb} set-exec -- /usr/bin/castled
    ${acb} write --overwrite ${acifile}
    ${acb} end

    rm -fr $tmpdir
}

publish() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    local aci=${RELEASE_DIR}/castle-${version}-${os}-${arch}.aci

    echo uploading aci file ${acifile}
}

action=$1
shift

${action} "$@"
