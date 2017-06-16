# Building Rook

Rook is composed of golang project and can be built directly with standard `golang` tools,
and storage software (like Ceph) that are built inside containers. We currently support
three different platforms for building:

  * Linux: most modern distros should work although most testing has been done on Ubuntu
  * Mac: macOS 10.6+ is supported

## Build Requirements

An Intel-based machine (recommend 2+ cores, 8+ GB of memory and 128GB of SSD).

The following tools are need on the host:
  - curl
  - docker (1.12+) or Docker for Mac (17+)
  - git
  - make
  - rsync (if you're using the build container on mac)

## Build

You can build Rook by simply running:

```
make -j4
```

This will build the Rook binaries and container images for the host platform.

You can also run the build for all supported platforms:

```
make -j4 build.all
```

See instructions below for setting up the build environment to support `arm` and `arm64` platforms.

Or if you wanted to build all release artifats run:

```
make -j4 release
```

Run `make help` for more options.

## Building inside the cross container

Offical Rook builds are done inside a build container. This ensures that we get a consistent build, test and release environment. To run the build inside the cross container run:

```
> build/run make -j4
```

The first run of `build/run` will build the container itself and could take a few
minutes to complete.

### Resetting the build container

If you're running the build container on the Mac using Docker for Mac, the build
container will rely on rsync to copy source to and from the host. To reset the build container and it's persistent volumes, you can run the below command. You shouldn't have to do this often unless something is broken or stale with your build container:

```
build/reset
```

## Building for other platforms

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

# Improving Build Speed

## Using CCache

C++ code can take time to compile. CCache can increase the speed of compilation. To enable make sure you have ccache installed:

```
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends ccache
```

when building container images we will use `CCACHE_DIR` or `${HOME}/.ccache` by default.

## Apt Package Caching

When building container images numerous apt-get operations are performed which can slow down the build significantly. To speed things up install the apt-cacher-ng package which will cache apt packages on the host.

On an Ubuntu host you can run the following:

```
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends apt-cacher-ng
```

on other platforms you can run apt-cacher-ng in a container:

```
cd images/apt-cacher && make
docker run -d --restart always --name apt-cacher-ng -p 3142:3142 -v ${HOME}/.apt-cacher:/var/cache/apt-cacher-ng apt-cacher-ng-amd64
```

Our base container image enables transparent proxy detection and will use the apt-cacher if available on the host. See images/base/Dockerfile for more information.

## Image Caching and Pruning

Doing a complete build of Rook and the dependent packages can take a long time (more than an hour on a typical macbook). To speed things up we rely heavily on image caching in docker. Docker support content-addressable images by default and we use that effectively when building images. Images are factored to increase reusability across builds. We also tag and timestamp images that should remain in the cache to help future builds.

Not all parts of our build, however, can rely on content-addressable image caching. For example to build Ceph using `docker build` would mean that we could not `ccache` and the build could take much longer. Instead we build Ceph by running the build process with `docker run` using a base image that has all the Ceph code and tools. To avoid building Ceph over and over, we create a container image with the output of the ceph build and add it to the docker image cache.

### Pruning the cache

To prune the number of cached images run `make prune`. There are two options that control the level of pruning performed:

- `PRUNE_HOURS` - the number of hours from when an image was last used (a cache hit) for it to be considered for pruning. The default is 48 hrs.
- `PRUNE_KEEP` - the minimum number of cached images to keep in the cache. Default is 24 images.
