# Cross Build Image

This directory builds a container image that can be used for cross building, and
also to minimize dependencies on the host platform.

## Building

To build the container run make in the directory.

## Pushing

To publish to docker hub run `make push`.

## Version

Be sure to update the version number in the `version` file when making changes
to the build container. We use semantic versioning for the build container.


