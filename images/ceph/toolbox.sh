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

ROOK_DIR="/var/lib/rook"
CEPH_CONFIG="/etc/ceph/ceph.conf"
MON_CONFIG="/etc/rook/mon-endpoints"
KEYRING_FILE="/etc/ceph/keyring"

# create a ceph config file in its default location so ceph/rados tools can be used
# without specifying any arguments
write_endpoints() {
    endpoints=$(cat ${MON_CONFIG})

    # filter out the mon names
    mon_endpoints=$(echo ${endpoints} | sed 's/[a-z]\+=//g')

    # filter out the legacy mon names
    mon_endpoints=$(echo ${mon_endpoints} | sed 's/rook-ceph-mon[0-9]\+=//g')

    DATE=$(date)
    echo "$DATE writing mon endpoints to ${CEPH_CONFIG}: ${endpoints}"
    cat <<EOF > ${CEPH_CONFIG}
[global]
mon_host = ${mon_endpoints}

[client.admin]
keyring = ${KEYRING_FILE}
EOF
}

# watch the endpoints config file and update if the mon endpoints ever change
watch_endpoints() {
    # get the timestamp for the target of the soft link
    real_path=$(realpath ${MON_CONFIG})
    initial_time=$(stat -c %Z ${real_path})
    while true; do
       real_path=$(realpath ${MON_CONFIG})
       latest_time=$(stat -c %Z ${real_path})

       if [[ "${latest_time}" != "${initial_time}" ]]; then
         write_endpoints
         initial_time=${latest_time}
       fi
       sleep 10
    done
}

# create the keyring file
cat <<EOF > ${KEYRING_FILE}
[client.admin]
key = ${ROOK_ADMIN_SECRET}
EOF

# write the initial config file
write_endpoints

# continuously update the mon endpoints if they fail over
watch_endpoints
