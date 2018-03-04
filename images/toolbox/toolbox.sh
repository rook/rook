#!/bin/bash -e

# Copyright 2018 The Rook Authors. All rights reserved.
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

CEPH_CONFIG="/etc/ceph/ceph.conf"
KEYRING_FILE="/etc/ceph/keyring"

# Create a ceph config file in its default location so ceph/rados tools can be used
# without specifying any arguments
write_endpoints() {
    echo "Writing mon endpoint to ${CEPH_CONFIG}"
    cat <<EOF > ${CEPH_CONFIG}
[global]
mon_host = rook-ceph-mon.${NAMESPACE}.svc:6790

[client.admin]
keyring = ${KEYRING_FILE}
EOF
}

if [[ -z ${ROOK_API_SERVER_ENDPOINT} ]]; then
    if [[ -z ${ROOK_API_SERVICE_HOST} ]] && [[ -z ${ROOK_API_SERVICE_PORT} ]]; then
        # When hostNetwork: true is most likely used, the DNS name for rook API
        # should be used for access
        ROOK_API_SERVICE_HOST="rook-api.${NAMESPACE}.svc"
        ROOK_API_SERVICE_PORT=8124
    fi
    # We have some rook env vars set, get client info from the rook API server and construct
    # a ceph config file from that
    ROOK_API_SERVER_ENDPOINT=${ROOK_API_SERVICE_HOST}:${ROOK_API_SERVICE_PORT}
fi

# Append to the bashrc file so that the rook tool can find the API server easily
# additionally add the kubernetes service discovery env vars to be sure they are set
cat <<EOF >> ~/.bashrc

export ROOK_API_SERVER_ENDPOINT=${ROOK_API_SERVER_ENDPOINT}
export ROOK_API_SERVICE_HOST=${ROOK_API_SERVICE_HOST}
export ROOK_API_SERVICE_PORT=${ROOK_API_SERVICE_PORT}
EOF

# Create the keyring file
cat <<EOF > ${KEYRING_FILE}
[client.admin]
key = ${ROOK_ADMIN_SECRET}
EOF

# Write the initial config file
write_endpoints

# Sleep endless
sleep infinity
