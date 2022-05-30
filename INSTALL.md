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
* helm

## Build

You can build the Rook binaries and all container images for the host platform by simply running the
command below. Building in parallel with the `-j` option is recommended.

```console
make -j4 build
```

Run `make help` for more options.

## CI Workflow

Every PR and every merge to master triggers the CI process in GitHub actions.
On every commit to PR and master the CI will build, run unit tests, and run integration tests.
If the build is for master or a release, the build will also be published to
[dockerhub.com](https://cloud.docker.com/u/rook/repository/list).

> Note that if the pull request title follows Rook's [contribution guidelines](https://rook.io/docs/rook/latest/Contributing/development-flow/#commit-structure), the CI will automatically run the appropriate test scenario. For example if a pull request title is "ceph: add a feature", then the tests for the Ceph storage provider will run. Similarly, tests will only run for a single provider with the "cassandra:" and "nfs:" prefixes.

## Building for other platforms

You can also run the build for all supported platforms:

```console
make -j4 build.all
```
