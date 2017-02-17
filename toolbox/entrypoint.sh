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

# attempt to create a ceph config file in its default location so ceph/rados tools can be used
# without specifying any arguments
if [[ -d ${ROOK_DIR} ]]; then
    # there is a rook directory, try to find one of rook's ceph config files and copy it
    ROOK_CONFIG=`find ${ROOK_DIR} -regex '.*mon[0-9]+/.*\.config' | head -1`
    if [[ ! -z ${ROOK_CONFIG} ]]; then
        mkdir -p /etc/ceph
        cp ${ROOK_CONFIG} ${CEPH_CONFIG}
    fi
elif [[ ! -z ${ROOK_API_SERVICE_HOST} ]] && [[ ! -z ${ROOK_API_SERVICE_PORT} ]]; then
    # we have some rook env vars set, get client info from the rook API server and construct
    # a ceph config file from that
    ROOK_API_SERVER_ENDPOINT=${ROOK_API_SERVICE_HOST}:${ROOK_API_SERVICE_PORT}

    # append to the bashrc file so that the rook tool can find the API server easily
    cat <<EOF >> ~/.bashrc

export ROOK_API_SERVER_ENDPOINT=${ROOK_API_SERVER_ENDPOINT}

EOF

    CLIENT_INFO=$(curl -s http://${ROOK_API_SERVER_ENDPOINT}/client)
    MON_ENDPOINTS=$(echo $CLIENT_INFO | jq -r '.monAddresses[]' | awk -F '/' 'BEGIN { ORS="," }; {print $1}' | sed 's/,$//')
    KEYRING_FILE="/etc/ceph/keyring"

    cat <<EOF > ${CEPH_CONFIG}
[global]
mon_host = ${MON_ENDPOINTS}

[client.admin]
keyring = ${KEYRING_FILE}
EOF

    cat <<EOF > ${KEYRING_FILE}
[client.admin]
key = ${ROOKD_ADMIN_SECRET}
EOF
fi

exec ${ARGS}
