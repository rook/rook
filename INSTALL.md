# Building Rook

Rook is composed of golang project and can be built directly with standard `golang` tools,
and storage software (like Ceph) that are built inside containers. We currently support
three different platforms for building:

  * Linux: most modern distros should work although most testing has been done on Ubuntu
  * Mac: macOS 10.6+ is supported

## Build Requirements

An Intel-based machine (recommend 2+ cores, 8+ GB of memory and 128GB of SSD). Inside your build environment (Docker for Mac or a VM), 6+ GB memory is also recommended.

The following tools are need on the host:
  - curl
  - docker (1.12+) or Docker for Mac (17+)
  - git
  - make
  - golang
  - rsync (if you're using the build container on mac)

## Build

You can build the Rook binaries and all container images for the host platform by simply running the
command below. Building in parallel with the `-j` option is recommended.

```
make -j4
```

Developers may often wish to make only images for a particular backend in their testing. This can
be done by specifying the `IMAGES` environment variable with `make` as exemplified below. Possible
values for are as defined by subdir names in the `/rook/images/` dir. Multiple images can be separated by a space.

```
make -j4 IMAGES='ceph' build
```

Run `make help` for more options.

## Building inside the cross container

Official Rook builds are done inside a build container. This ensures that we get a consistent build, test and release environment. To run the build inside the cross container run:

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

You can also run the build for all supported platforms:

```
make -j4 build.all
```

Or from the cross container:

```
build/run make -j4 build.all
```

Currently, only `amd64` platform supports this kind of 'cross' build. In order to make `build.all`
succeed, we need to follow the steps specified by the below section `Building for other platforms`
first.

We suggest to use native `build` to create the binaries and container images on `arm{32,64}`
platform, but if you do want to build those `arm{32,64}` binaries and images on `amd64` platform
with `build.all` command, please make sure the multi-arch feature is supported. To test run the
following:

```
> docker run --rm -ti arm32v7/ubuntu uname -a
Linux bad621a75757 4.8.0-58-generic #63~16.04.1-Ubuntu SMP Mon Jun 26 18:08:51 UTC 2017 armv7l armv7l armv7l GNU/Linux

> docker run --rm -ti arm64v8/ubuntu uname -a
Linux f51ea93e76a2 4.8.0-58-generic #63~16.04.1-Ubuntu SMP Mon Jun 26 18:08:51 UTC 2017 aarch64 aarch64 aarch64 GNU/Linux
```

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

# Improving Build Speed

## Image Caching and Pruning

Doing a complete build of Rook and the dependent packages can take a long time (more than an hour on a typical macbook). To speed things up we rely heavily on image caching in docker. Docker support content-addressable images by default and we use that effectively when building images. Images are factored to increase reusability across builds. We also tag and timestamp images that should remain in the cache to help future builds.

### Pruning the cache

To prune the number of cached images run `make prune`. There are two options that control the level of pruning performed:

- `PRUNE_HOURS` - the number of hours from when an image was last used (a cache hit) for it to be considered for pruning. The default is 48 hrs.
- `PRUNE_KEEP` - the minimum number of cached images to keep in the cache. Default is 24 images.

## CI workflow and options
Every PR and every merge to master triggers the CI process in [Jenkins](http://jenkins.rook.io).
The Jenkins CI will build, run unit tests, run integration tests and Publish artifacts- On every commit to PR and master.
If any of the CI stages fail, then the process is aborted and no artifacts are published.
On every successful build Artifacts are pushed to a [s3 bucket](https://release.rook.io/). On every successful master build,
images are uploaded to docker hub in addition.

During Integration tests phase, all End to End Integration tests under [/tests/integration](/tests/integration) are run.
It may take a while to run all Integration tests. Based on nature of the PR, it may not be required to run full regression
Or users may want to skip build all together for trivial changes like documentation changes. Based on the PR body text,Jenkins will skip the build or skip some tests

1. [skip ci] - if this text is found in the body of PR, then Jenkins will skip the build process and accept the commit
2. [smoke only] - if this text is found in the body of PR, then Jenkins will only run Smoke Test during integration test phase

The above flags work only on PRs,The full regression is run on every merge to master.
