#!/usr/bin/env -S bash -e
set -u
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

BUILD_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd -P)
SHA256CMD=${SHA256CMD:-shasum -a 256}

DOCKERCMD=${DOCKERCMD:-docker}

export scriptdir
scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export OUTPUT_DIR=${BUILD_ROOT}/_output
export WORK_DIR=${BUILD_ROOT}/.work
export CACHE_DIR=${BUILD_ROOT}/.cache

function ver() {
    local full_ver maj min bug build
    full_ver="$1" # functions should name input params for easier understanding
    maj="$(echo "${full_ver}" | cut -f1 -d'.')" # when splitting a param, name the components for easier understanding
    min="$(echo "${full_ver}" | cut -f2 -d'.')"
    bug="$(echo "${full_ver}" | cut -f3 -d'.')"
    build="$(echo "${full_ver}" | cut -f4 -d'.')"
  printf "%d%03d%03d%03d" "${maj}" "${min}" "${bug}" "${build}"
}

function check_git() {
    # git version 2.6.6+ through 2.8.3 had a bug with submodules. this makes it hard
    # to share a cloned directory between host and container
    # see https://github.com/git/git/blob/master/Documentation/RelNotes/2.8.3.txt#L33
    local gitversion
    gitversion=$(git --version | cut -d" " -f3)

    if (( $(ver "${gitversion}") > $(ver 2.6.6) && $(ver "${gitversion}") < $(ver 2.8.3) )); then
        echo WARN: you are running git version "${gitversion}" which has a bug related to relative
        echo WARN: submodule paths. Please consider upgrading to 2.8.3 or later
    fi
}
