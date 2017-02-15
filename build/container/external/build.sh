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

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# set the working directory
if [[ -z ${WORKDIR} ]]; then
  WORKDIR=${scriptdir}
fi

builddir=${WORKDIR}/build
installdir=${WORKDIR}/install
downloaddir=${WORKDIR}/download

for p in x86_64-linux-gnu aarch64-linux-gnu; do
    rm -fr ${builddir}/${p}
    rm -fr ${installdir}/${p}
    cmake \
      -H${scriptdir} \
      -B${builddir}/${p} \
      -DEXTERNAL_DOWNLOAD_DIR=${downloaddir} \
      -DCMAKE_INSTALL_PREFIX=${installdir}/${p} \
      -DCMAKE_TOOLCHAIN_FILE=${scriptdir}/toolchain/${p}.cmake \
      -DEXTERNAL_LOGGING=ON \
      -DCMAKE_BUILD_TYPE=RelWithDebInfo \
      -DCMAKE_POSITION_INDEPENDENT_CODE=ON
    make -C ${builddir}/${p} V=1 $@
done
