# Cross Building

This directory builds a container that can be used for cross building, and
also to minimize dependencies on the host platform.

## Building

To build the container run make in the directory.

## Publishing

To publish to docker hub run ```make publish```. Be sure to update the
version number in the ```version``` file. We use semantic versioning for
the build container.


