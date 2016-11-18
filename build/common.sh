#!/bin/bash -e

# Copyright 2016 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

source_repo=github.com/rook/rook

container_version=$(cat ${scriptdir}/cross-image/version)
container_image=quay.io/rook/cross-build:${container_version}
container_volume=cross-build-volume
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

function start_rsync_container() {
    docker run \
        -d \
        -e OWNER=root \
        -e GROUP=root \
        -e MKDIRS="/volume/src/${source_repo}" \
        -p ${rsync_port}:873 \
        --entrypoint /bin/bash \
        -v ${container_volume}:/volume \
        ${container_image} \
        /build/rsyncd.sh
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

function stop_rsync_container() {
    id=$1

    docker stop ${id} &> /dev/null || true
    docker rm ${id} &> /dev/null || true
}

function run_rsync() {

    # run the container as an rsyncd daemon so that we can copy the
    # source tree to the container volume.
    id=$(start_rsync_container)

    # wait for rsync to come up
    wait_for_rsync || stop_rsync_container ${id}

    for pair in "$*" ; do
        src="${pair%%-->*}"
        dst="${pair##*-->}"

        rsync \
            --archive \
            --delete \
            --prune-empty-dirs \
            --filter='- /.work/' \
            --filter='- /.glide/' \
            --filter='- /.vscode/' \
            --filter='- /bin/' \
            --filter='- /release/' \
            $src $dst || { stop_rsync_container ${id}; return 1; }
    done


    stop_rsync_container ${id}
}
