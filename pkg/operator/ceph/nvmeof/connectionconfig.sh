#!/usr/bin/env bash
set -xEeuo pipefail

cat << EOF > /etc/ceph/ceph.conf
[global]
mon_host = ${ROOK_CEPH_MON_HOST}
log_to_stderr = true
keyring = /etc/ceph/keyring
EOF

cp /tmp/ceph/keyring /etc/ceph

chmod 444 /etc/ceph/ceph.conf
chmod 440 /etc/ceph/keyring

# ARGS: all args are assumed to be 'ceph' CLI args
# Use "$@" to pass through Ceph CLI arguments (e.g., --connect-timeout, --mon-host, --keyring, etc.)
# This allows easy passthru of arguments set by Rook's methods for Ceph CLI options

# Replace the placeholders with bash pattern substitution rather than sed, so
# that no value is parsed as a sed script. Each replacement is quoted so that
# bash 5.2+ does not turn '&' in it into the matched placeholder. Tracing is
# paused so the config is not echoed after every substitution.
set +x
conf="$(< /config/nvmeof.conf)"
conf="${conf//@@POD_NAME@@/"${GATEWAY_NAME}"}"
conf="${conf//@@ANA_GROUP@@/"${ANA_GROUP}"}"
conf="${conf//@@POD_IP@@/"${POD_IP}"}"
printf '%s\n' "${conf}" > /etc/ceph/nvmeof.conf
set -x

ceph "$@" nvme-gw create "${GATEWAY_NAME}" "${POOL_NAME}" "${ANA_GROUP}" || true
ceph "$@" nvme-gw show "${POOL_NAME}" "${ANA_GROUP}" || true
