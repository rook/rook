#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"


# git version 2.6.6+ through 2.8.3 had a bug with submodules
# see https://github.com/git/git/blob/master/Documentation/RelNotes/2.8.3.txt#L33
function ver { 
    printf "%d%03d%03d%03d" $(echo "$1" | tr '.' ' ') 
}
gitversion=$(git --version | cut -d" " -f3)
if (( $(ver ${gitversion}) > $(ver 2.6.6) && $(ver ${gitversion}) < $(ver 2.8.3) )); then
    echo WARN: your running git version ${gitversion} which has a realted to relative
    echo WARN: submodule paths. Please consider upgrading to 2.8.3 or later
fi

docker run \
    -v ${HOME}/.netrc:/root/.netrc \
    -v ${scriptdir}/..:/go/src/github.com/quantum/castle \
    quantum/castle-build \
    "$*"
