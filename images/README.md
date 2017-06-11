# Images

This folder contains all images used by the Rook project:

  - base - small base Ubuntu image used as the base for other images
  - ceph - images that build ceph
  - cross - an image that contains tools for cross building
  - cross-gnu - an image that contains gnu tools for cross compiling
  - cross-go - an image that contains gnu tools and go tools for cross compiling
  - rookd - an image that contains rookd, ceph and other tools

# Building Images

## Build Requirements

A capable machine (2+ cores, 8+ GB of memory):

  * MacOS: you can use Docker for Mac or your own docker-machine
  * Linux: any distro with Docker 1.12+

We do not currently support building on a remote docker host.

## Building for the host platform

Run `make` or `make build` will build all the images for the host platform. Note that this
will build all images and cache them (see Image Caching below).

## Building for other platforms

To build for all supported platforms run `make build.all`.

In order to build container images for these platforms we rely on cross-compilers and QEMU. Cross compiling is much faster than QEMU and so we lean heavily on it.

In order for QEMU to work inside docker containers you need to do a few things on
the linux host. If you are using a recent Docker for Mac build you can skip this section, since they added support for binfmt and multi-arch docker builds.

On an Ubuntu machine with a 4.8+ kernel you need to run install the following:

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
ExecStartPre=/usr/bin/docker pull hypriot/qemu-register
ExecStart=/usr/bin/docker run --rm --privileged hypriot/qemu-register
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl enable update-binfmt.service
```

### Checking if multi-arch is supported

To test run the following:

```
> docker run --rm -ti armhf/ubuntu uname -a
Linux dece9819612d 4.10.0-21-generic #23-Ubuntu SMP Fri Apr 28 16:14:22 UTC 2017 armv7l armv7l armv7l GNU/Linux

> docker run --rm -ti aarch64/ubuntu uname -a
Linux 7ed5f0d0b618 4.10.0-21-generic #23-Ubuntu SMP Fri Apr 28 16:14:22 UTC 2017 aarch64 aarch64 aarch64 GNU/Linux
```

# Image Caching and Pruning

Doing a complete build of Rook and the storage packages it depends on can take a long time (more than an hour on a typical macbook). To speed things up we rely heavily on image caching in docker. Docker support content-addressable images by default and we use that
effectively when building images. Images are factored to increase reusability across
builds. We also tag and timestamp images that should remain in the cache to help future builds.

Not all parts of our build, however, can rely on content-addressable image caching. For example to build Ceph using `docker build` would mean that we could not `ccache` and the build could take much longer. Instead we build Ceph by running the build process with
`docker run` using a base image that has all the Ceph code and tools. To avoid building
Ceph over and over, we create a container image with the output of the ceph build and add it to the docker image cache.

## Pruning the cache

To prune the number of cached images run `make prune`. There are two options that control the level of pruning performed:

- `PRUNE_HOURS` - the number of hours from when an image was last used (a cache hit) for it to be considered for pruning. The default is 48 hrs.
- `PRUNE_KEEP` - the minimum number of cached images to keep in the cache. Default is 24 images.

# Apt Package Caching

When building images numerous apt-get operations are performed which can slow down the build significantly. To speed things up install the apt-cacher-ng package which will
cache apt packages on the host.

```
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends apt-cacher-ng
```

Our base image enables transparent proxy detection and will use the apt-cacher if available on the host. See images/base/Dockerfile for more information.

