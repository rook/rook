#!/bin/bash -e

source_repo=github.com/rook/rook

container_version=$(cat ${scriptdir}/cross-image/version)
container_image=quantum/castle-cross-build:${container_version}
container_name=cross-build
container_volume=${container_name}-volume
rsync_port=10873

function ver() {
    printf "%d%03d%03d%03d" $(echo "$1" | tr '.' ' ')
}

function check_git() {
    # git version 2.6.6+ through 2.8.3 had a bug with submodules. this makes it hard
    # to share a cloned directory between host and container
    # see https://github.com/git/git/blob/master/Documentation/RelNotes/2.8.3.txt#L33
    gitversion=$(git --version | cut -d" " -f3)
    if (( $(ver ${gitversion}) > $(ver 2.6.6) && $(ver ${gitversion}) < $(ver 2.8.3) )); then
        echo WARN: your running git version ${gitversion} which has a bug realted to relative
        echo WARN: submodule paths. Please consider upgrading to 2.8.3 or later
    fi
}

function wait_for_rsync() {
    # wait for rsync to come up
    tries=100
    while (( ${tries} > 0 )) ; do
        if rsync "rsync://localhost:${rsync_port}/"  &> /dev/null ; then
            return 0
        fi
        tries=$(( ${tries} - 1 ))
        sleep 0.1
    done
    echo ERROR: rsyncd did not come up >&2
    exit 1
}
