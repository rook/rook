#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

action=$1
shift

mkdir -p ${RELEASE_DIR}

case ${action} in
    build|publish)
        platform=$1
        shift

        flavor=$1
        shift

        ${scriptdir}/${flavor}.sh ${action} ${platform%_*} ${platform#*_}
        ;;

    check)
        github_check_release "$@"
        ;;

    *)
        echo "unknown action ${action}"
        exit 1
        ;;
esac
