# Building Rook

While it is possible to build Rook directly on your host, we highly discourage it.
Rook has many dependencies (mostly through Ceph) and it might be hard to keep
up with the different versions across distros. Instead we recommend that you use
the build process that runs inside a Docker container. It results in a consistent
build, test and release environment.

## Requirements

A capable machine (2+ cores, 8+ GB of memory) with Docker installed locally:

  * MacOS: you can use Docker for Mac or your own docker-machine
  * Linux: any distro with a recent version of Docker would work

We do not currently support building on a remote docker host.

## Update the git submodules

Rook has many git submodules, and before you can
build Rook you need to initialize them:

```
git submodule sync --recursive
git submodule update --recursive --init
```

## Using the Build Container

To run a command inside the container:

```
> build/run <command>
```

This will run  `<command>` from inside the container where all tools and dependencies
have been installed and configured. The current directory is set to the source directory
which is bind mounted (or rsync'd on some platforms) inside the container.

The first run of `build/run` will pull the container (which can take a while).

If you don't pass a command you get an interactive shell inside the container.

```
> build/run
rook@moby:~/go/src/github.com/rook/rook$
```

## Building Rook

Now you can build in the container normally, like so:

```
build/run make -j4
```

This will build just the binaries and copy them to the applicable subfolder of ./bin.

See [Makefile](Makefile.md) for more build options.


## Updating the Build container

To modify the build container change the Dockerfile and/or associated scripts. Also bump
the version in `build/container/version`. We require a version bump for any change
in order to use the correct version of the container across releases and branches.
To create a new build of the container run:

```
cd build/container
make
```

If all looks good you can publish it by calling `make publish`. Note that you need
quay.io credentials to push a new build.

Here's a rough sketch of the proposed approach somewhat influenced by how CoreOS is released.

## Resetting the Container

To reset the build container and it's persistent volumes, you can run the below command.
You shouldn't have to do this often unless something is broken or stale with your
build container:

```
build/reset
```
