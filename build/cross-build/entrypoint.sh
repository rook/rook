#!/bin/bash -e

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
    chown -R $BUILDER_UID:$BUILDER_GID $HOME/*
    cd ${HOME}/castle
    echo "export PATH=$PATH" > $HOME/.bash_profile
    exec chpst -u :$BUILDER_UID:$BUILDER_GID "$@"
else
    exec "$@"
fi
