# Building Rook

Rook is composed of a golang project and can be built directly with standard `golang` tools,
and storage software (like Ceph) that are built inside containers. We currently support
these platforms for building:

* Linux: most modern distributions should work although most testing has been done on Ubuntu
* Mac: macOS 10.6+ is supported

## Build Requirements

Recommend 2+ cores, 8+ GB of memory and 128GB of SSD. Inside your build environment (Docker for Mac or a VM), 2+ GB memory is also recommended.

The following tools are need on the host:

* curl
* docker (1.12+) or Docker for Mac (17+)
* git
* make
* golang
* rsync (if you're using the build container on mac)

## Build

You can build the Rook binaries and all container images for the host platform by simply running the
command below. Building in parallel with the `-j` option is recommended.

```console
make -j4
```

Developers may often wish to make only images for a particular backend in their testing. This can
be done by specifying the `IMAGES` environment variable with `make` as exemplified below. Possible
values for are as defined by sub-directory names in the `/rook/images/` dir. Multiple images can be separated by a space.

```console
make -j4 IMAGES='ceph' build
```

Run `make help` for more options.

## CI Workflow

Every PR and every merge to master triggers the CI process in Github actions.
On every commit to PR and master the CI will build, run unit tests, and run integration tests.
If the build is for master or a release, the build will also be published to
[dockerhub.com](https://cloud.docker.com/u/rook/repository/list).

> Note that if the pull request title follows Rook's [contribution guidelines](https://rook.io/docs/rook/latest/development-flow.html#commit-structure), the CI will automatically run the appropriate test scenario. For example if a pull request title is "ceph: add a feature", then the tests for the Ceph storage provider will run. Similarly, tests will only run for a single provider with the "cassandra:" and "nfs:" prefixes.

## Building for other platforms

You can also run the build for all supported platforms:

```console
make -j4 build.all
```

Or from the cross container:

```console
build/run make -j4 build.all
```

Currently, only `amd64` platform supports this kind of 'cross' build. In order to make `build.all`
succeed, we need to follow the steps specified by the below section `Building for other platforms`
first.

We suggest to use native `build` to create the binaries and container images on `arm{32,64}`
platform, but if you do want to build those `arm{32,64}` binaries and images on `amd64` platform
with `build.all` command, please make sure the multi-arch feature is supported. To test run the
following:

```console
> docker run --rm -ti arm32v7/ubuntu uname -a
Linux bad621a75757 4.8.0-58-generic #63~16.04.1-Ubuntu SMP Mon Jun 26 18:08:51 UTC 2017 armv7l armv7l armv7l GNU/Linux

> docker run --rm -ti arm64v8/ubuntu uname -a
Linux f51ea93e76a2 4.8.0-58-generic #63~16.04.1-Ubuntu SMP Mon Jun 26 18:08:51 UTC 2017 aarch64 aarch64 aarch64 GNU/Linux
```

In order to build container images for these platforms we rely on cross-compilers and QEMU. Cross compiling is much faster than QEMU and so we lean heavily on it.

In order for QEMU to work inside docker containers you need to do a few things on
the linux host. If you are using a recent Docker for Mac build you can skip this section, since they added support for binfmt and multi-arch docker builds.

On an Ubuntu machine with a 4.8+ kernel you need to run install the following:

```console
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
    binfmt-support
    qemu-user-static
```

You also need to run the following on every boot:

```console
docker run --rm --privileged hypriot/qemu-register
```

you can install a systemd unit to help with this if you'd like, for example:

```console
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
