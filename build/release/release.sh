#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${scriptdir}/common.sh

run_platforms() {
    local action=${1}
    local flavor=${2}

    for p in ${RELEASE_CLIENT_SERVER_PLATFORMS}; do
        ${scriptdir}/${flavor}.sh ${action} both ${p%_*} ${p#*_}
    done
    for p in ${RELEASE_CLIENT_ONLY_PLATFORMS}; do
        ${scriptdir}/${flavor}.sh ${action} client ${p%_*} ${p#*_}
    done
}

action=$1
shift

case ${action} in
    build|publish)
        flavor=$1
        shift

        run_platforms ${action} ${flavor} "$@"
        ;;

    check)
        github_check_release "$@"
        ;;

    *)
        echo "unknown action ${action}"
        exit 1
        ;;
esac
