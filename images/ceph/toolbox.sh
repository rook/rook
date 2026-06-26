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

# inputs
ENDPOINT_MOUNT="/etc/rook/mon-endpoints"
KEYRING_MOUNT="/var/lib/rook-ceph-mon/secret.keyring"
CONFIG_OVERRIDE_MOUNT="/etc/rook-config-override/config"

# outputs
CEPH_CONFIG="/etc/ceph/ceph.conf"
KEYRING_FILE="/etc/ceph/keyring"

config_override_exists() {
  [ -f "${CONFIG_OVERRIDE_MOUNT}" ] && [ -s "${CONFIG_OVERRIDE_MOUNT}" ]
}

# create/update the ceph keyring file
write_keyring() {
  DATE=$(date)

  # read the secret from an env var (for backward compatibility), or from the secret file
  ceph_secret=${ROOK_CEPH_SECRET}
  if [[ "$ceph_secret" == "" ]]; then
    ceph_secret=$(cat ${KEYRING_MOUNT})
  fi

  echo "$DATE writing keyring to ${KEYRING_FILE}"
  cat <<EOF > ${KEYRING_FILE}
[${ROOK_CEPH_USERNAME}]
key = ${ceph_secret}
EOF

} # write_keyring()

# create/update the ceph config file in its default location so ceph/rados tools can be used
# without specifying any arguments
write_conf() {
  DATE=$(date)

  # filter out the mon names
  # external cluster can have numbers or hyphens in mon names, handling them in regex
  # shellcheck disable=SC2001
  mon_endpoints=$(cat ${ENDPOINT_MOUNT}| sed 's/[a-z0-9_-]\+=//g')

  config_override=""
  if config_override_exists; then
    echo "$DATE merging config override from ${CONFIG_OVERRIDE_MOUNT}"
    config_override=$(cat "${CONFIG_OVERRIDE_MOUNT}")
  fi

  echo "$DATE writing mon endpoints to ${CEPH_CONFIG}: ${mon_endpoints}"
  cat << EOF > ${CEPH_CONFIG}
[global]
mon_host = ${mon_endpoints}

[client.admin]
keyring = ${KEYRING_FILE}

${config_override}
EOF

} # write_conf()

# watch the ceph config directory and update if the mon endpoints or keyring change
watch_etc_ceph() {
  # get the timestamp for the targets of the soft links
  keyring_mount_path=$(realpath "${KEYRING_MOUNT}")
  keyring_init_time=$(stat -c %Z "${keyring_mount_path}")

  endpoint_mount_path=$(realpath "${ENDPOINT_MOUNT}")
  endpoint_init_time=$(stat -c %Z "${endpoint_mount_path}")

  override_mount_path=""
  override_init_time=""
  if config_override_exists; then
    override_mount_path=$(realpath "${CONFIG_OVERRIDE_MOUNT}")
    override_init_time=$(stat -c %Z "${override_mount_path}")
  fi

  while true; do
    sleep 10
    DATE=$(date)

    # keyring file
    keyring_mount_path=$(realpath "${KEYRING_MOUNT}")
    keyring_latest_time="$(stat -c %Z "${keyring_mount_path}")"

    if [ "${keyring_latest_time}" != "${keyring_init_time}" ]; then
      echo "$DATE keyring changed"
      write_keyring
      keyring_init_time="${keyring_latest_time}"
    fi

    # ceph.conf file / config override
    endpoint_mount_path=$(realpath "${ENDPOINT_MOUNT}")
    endpoint_latest_time=$(stat -c %Z "${endpoint_mount_path}")

    override_mount_path=""
    override_latest_time=""
    if config_override_exists; then
      override_mount_path=$(realpath "${CONFIG_OVERRIDE_MOUNT}")
      override_latest_time=$(stat -c %Z "${override_mount_path}")
    fi

    need_conf_update=false

    if [ "${endpoint_latest_time}" != "${endpoint_init_time}" ]; then
      echo "$DATE mon endpoints changed"
      need_conf_update=true
    fi

    if [ "${override_latest_time}" != "${override_init_time}" ]; then
      echo "$DATE config override changed"
      need_conf_update=true
    fi

    if $need_conf_update; then
      write_conf
      endpoint_init_time="${endpoint_latest_time}"
      override_init_time="${override_latest_time}"
    fi
  done
}

# write the initial config files
write_keyring
write_conf

# continuously update the config files for mon failover and/or cephx key rotation
if [ "$1" != "--skip-watch" ]; then
  watch_etc_ceph
fi
