#!/usr/bin/env -S bash -e

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

CEPH_CONFIG="/etc/ceph/ceph.conf"
MON_CONFIG="/etc/rook/mon-endpoints"
KEYRING_FILE="/etc/ceph/keyring"
CONFIG_OVERRIDE="/etc/rook-config-override/config"

# create a ceph config file in its default location so ceph/rados tools can be used
# without specifying any arguments
write_endpoints() {
  endpoints=$(cat ${MON_CONFIG})

  # filter out the mon names
  # external cluster can have numbers or hyphens in mon names, handling them in regex
  # shellcheck disable=SC2001
  mon_endpoints=$(echo "${endpoints}"| sed 's/[a-z0-9_-]\+=//g')

  DATE=$(date)
  echo "$DATE writing mon endpoints to ${CEPH_CONFIG}: ${endpoints}"
    cat <<EOF > ${CEPH_CONFIG}
[global]
mon_host = ${mon_endpoints}

[client.admin]
keyring = ${KEYRING_FILE}
EOF

  # Merge the config override if it exists and is not empty
  if [ -f "${CONFIG_OVERRIDE}" ] && [ -s "${CONFIG_OVERRIDE}" ]; then
    echo "$DATE merging config override from ${CONFIG_OVERRIDE}"
    echo "" >> ${CEPH_CONFIG}
    cat ${CONFIG_OVERRIDE} >> ${CEPH_CONFIG}
  fi
}

# watch the endpoints config file and update if the mon endpoints ever change
watch_endpoints() {
  # get the timestamp for the target of the soft link
  real_path=$(realpath ${MON_CONFIG})
  initial_time=$(stat -c %Z "${real_path}")
  while true; do
    real_path=$(realpath ${MON_CONFIG})
    latest_time=$(stat -c %Z "${real_path}")

    if [[ "${latest_time}" != "${initial_time}" ]]; then
      write_endpoints
      initial_time=${latest_time}
    fi

    sleep 10
  done
}

# read the secret from an env var (for backward compatibility), or from the secret file
ceph_secret=${ROOK_CEPH_SECRET}
if [[ "$ceph_secret" == "" ]]; then
  ceph_secret=$(cat /var/lib/rook-ceph-mon/secret.keyring)
fi

# create the keyring file
cat <<EOF > ${KEYRING_FILE}
[${ROOK_CEPH_USERNAME}]
key = ${ceph_secret}
EOF

# write the initial config file
write_endpoints

# continuously update the mon endpoints if they fail over
if [ "$1" != "--skip-watch" ]; then
  watch_endpoints
fi
