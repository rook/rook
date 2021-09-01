#!/bin/bash -e
set -e
set -x

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

ARGS=( "$@" )
if [ $# -eq 0 ]; then
    ARGS=( /bin/bash )
fi

BUILDER_USER=${BUILDER_USER:-rook}
BUILDER_GROUP=${BUILDER_GROUP:-rook}
BUILDER_UID=${BUILDER_UID:-1000}
BUILDER_GID=${BUILDER_GID:-1000}

groupadd -o -g "$BUILDER_GID" "$BUILDER_GROUP" 2> /dev/null
useradd -o -m -g "$BUILDER_GID" -u "$BUILDER_UID" "$BUILDER_USER" 2> /dev/null
echo "$BUILDER_USER    ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers
export HOME=/home/${BUILDER_USER}
echo "127.0.0.1 $(cat /etc/hostname)" >> /etc/hosts
[[ -S /var/run/docker.sock ]] && chmod 666 /var/run/docker.sock
chown -R "$BUILDER_UID":"$BUILDER_GID" "$HOME"
exec chpst -u :"$BUILDER_UID":"$BUILDER_GID" "${ARGS[@]}"
