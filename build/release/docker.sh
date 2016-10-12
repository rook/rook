#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

container=quantum/castle

build() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    tmpdir=$(mktemp -d)
    trap "rm -fr $tmpdir" EXIT

    layout_root $os $arch $tmpdir

    cat <<EOF > $tmpdir/Dockerfile
FROM scratch
COPY root /
ENTRYPOINT ["/usr/bin/castled"]
EOF

    tag=${container}-${arch}:${version}
    if [[ ${arch} == "amd64" ]]; then
        tag=${container}:${version}
    fi

    echo building docker container ${tag}
    docker build -t ${tag} $tmpdir
    rm -fr $tmpdir
}

publish() {
    local type=$1
    local os=$2
    local arch=$3
    local version=${RELEASE_VERSION}

    [[ ${type} == "both" ]] || return 0

    tag=${container}-${arch}:${version}

    if [[ ${arch} == "amd64" ]]; then
        tag=${container}:${version}
    fi

    echo pushing docker container ${tag}
    docker push ${tag}
}

action=$1
shift

${action} "$@"
