#!/usr/bin/env bash
set -xEeuo pipefail

cat << EOF > /etc/ceph/ceph.conf
[global]
mon_host = ${CEPH_MON_HOST}
log_to_stderr = true
keyring = /etc/ceph/keyring
EOF

cp /tmp/ceph/keyring /etc/ceph

chmod 444 /etc/ceph/ceph.conf
chmod 440 /etc/ceph/keyring

sed -e "s/@@POD_NAME@@/${POD_NAME}/g" \
    -e "s/@@ANA_GROUP@@/${ANA_GROUP}/g" \
    -e "s/@@POD_IP@@/${POD_IP}/g" \
    < /config/nvmeof.conf > /etc/ceph/nvmeof.conf

ceph nvme-gw create ${POD_NAME} {{ .PoolName }} ${ANA_GROUP}
ceph nvme-gw show {{ .PoolName }} ${ANA_GROUP}
