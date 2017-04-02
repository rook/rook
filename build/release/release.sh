#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

action=$1
shift

mkdir -p ${RELEASE_DIR}

case ${action} in
    build|publish|promote|cleanup)
        platform=$1
        shift

        flavor=$1
        shift

        ${scriptdir}/${flavor}.sh ${action} ${platform%_*} ${platform#*_}
        ;;

    init)
        if check_release_version; then
            echo ${RELEASE_VERSION} can not be promoted. Must build from a tag like v0.4.0.
            exit 1
        fi
        github_create_release
        ;;

    *)
        echo "unknown action ${action}"
        exit 1
        ;;
esac
