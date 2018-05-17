#!/bin/sh -e

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

VOLUME=${VOLUME:-/volume}
ALLOW=${ALLOW:-192.168.0.0/16 172.16.0.0/12 10.0.0.0/8}
OWNER=${OWNER:-nobody}
GROUP=${GROUP:-nogroup}

if [[ "${GROUP}" != "nogroup" && "${GROUP}" != "root" ]]; then
    groupadd -g ${GROUP} rsync
fi

if [[ "${OWNER}" != "nobody" && "${OWNER}" != "root" ]]; then
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
