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

# If we are running docker natively, we want to create a user in the container
# with the same UID and GID as the user on the host machine, so that any files
# created are owned by that user. Without this they are all owned by root.
# If we are running from boot2docker, this is not necessary.
# The dockcross script sets the BUILDER_UID and BUILDER_GID vars.
if [[ -n $BUILDER_UID ]] && [[ -n $BUILDER_GID ]]; then
    groupadd -o -g $BUILDER_GID $BUILDER_GROUP 2> /dev/null
    useradd -o -m -g $BUILDER_GID -u $BUILDER_UID $BUILDER_USER 2> /dev/null
    echo "$BUILDER_USER    ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers
    export HOME=/home/${BUILDER_USER}
    [[ -S /var/run/docker.sock ]] && chown $BUILDER_UID:$BUILDER_GID /var/run/docker.sock
    chown -R $BUILDER_UID:$BUILDER_GID $HOME
    exec chpst -u :$BUILDER_UID:$BUILDER_GID ${ARGS}
else
    exec ${ARGS}
fi
