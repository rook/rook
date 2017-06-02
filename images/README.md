# Images

This folder contains all images used by the Rook project:

  - base - this is a small base Ubuntu image

# Versioning

Be sure to bump the version number everytime you make a change to the container,
regardless of how small that change.

# Build Requirements

A capable machine (2+ cores, 8+ GB of memory) with Docker installed locally:

  * MacOS: you can use Docker for Mac or your own docker-machine
  * Linux: any distro with a recent version of Docker would work

We do not currently support building on a remote docker host.

## Cross Building Containers

If you are using a recent Docker for Mac build you can skip this section, since
they added support for binfmt and multi-arch docker builds.

If you are planning on cross building the arm and arm64 containers you must
install QEMU and enable binfmt support. On an Ubuntu machine with a 4.8+ kernel
you need to run install the following:

```
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
    binfmt-support
    qemu-user-static
```

You also need to run the following on every boot:

```
docker run --rm --privileged hypriot/qemu-register
```

you can install a systemd unit to help with this if you'd like, for example:

```
cat <<EOF > /etc/systemd/system/update-binfmt.service
[Unit]
After=docker.service

[Service]
Type=oneshot
ExecStart=/usr/bin/docker run --rm --privileged hypriot/qemu-register
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl enable update-binfmt.service
```

## Checking if multi-arch is supported

To test run the following:

```
> docker run --rm -ti armhf/alpine uname -a
Linux 6ab4ed3ad07a 4.10.0-20-generic #22-Ubuntu SMP Thu Apr 20 09:22:42 UTC 2017 armv7l Linux

> docker run --rm -ti aarch64/alpine uname -a
Linux 758d8063d612 4.10.0-20-generic #22-Ubuntu SMP Thu Apr 20 09:22:42 UTC 2017 aarch64 Linux
```
