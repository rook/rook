#!/bin/bash -e

VOLUME=${VOLUME:-/volume}
ALLOW=${ALLOW:-192.168.0.0/16 172.16.0.0/12 10.0.0.0/8}
OWNER=${OWNER:-nobody}
GROUP=${GROUP:-nogroup}

if [ "${GROUP}" != "nogroup" ]; then
        groupadd -g ${GROUP} rsync
fi
if [ "${OWNER}" != "nobody" ]; then
        groupadd -u ${OWNER} -G rsync rsync
fi

chown "${OWNER}:${GROUP}" "${VOLUME}"

[ -f /etc/rsyncd.conf ] || cat <<EOF > /etc/rsyncd.conf
uid = ${OWNER}
gid = ${GROUP}
use chroot = yes
log file = /dev/stdout
reverse lookup = no
[volume]
    hosts deny = *
    hosts allow = ${ALLOW}
    read only = false
    path = ${VOLUME}
    comment = volume
EOF

for dir in ${MKDIRS}; do
    mkdir -p ${dir}
    chown "${OWNER}:${GROUP}" ${dir}
done

exec /usr/bin/rsync --no-detach --daemon --config /etc/rsyncd.conf "$@"
