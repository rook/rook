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

ARGS="$@"
if [ $# -eq 0 ]; then
    ARGS=/bin/bash
fi

# find one of rook's ceph config files and copy it to the default location so ceph/rados tools
# can be used without specifying a config file path
ROOK_DIR="/var/lib/rook"
if [[ -d ${ROOK_DIR} ]]; then
    ROOK_CONFIG=`find ${ROOK_DIR} -regex '.*mon[0-9]+/.*\.config' | head -1`
    if [[ ! -z ${ROOK_CONFIG} ]]; then
        mkdir -p /etc/ceph
        cp ${ROOK_CONFIG} /etc/ceph/ceph.conf
    fi
fi

exec ${ARGS}