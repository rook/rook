#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $scriptdir/build-container
docker build -t quantum/castle-build .
docker push quantum/castle-build
