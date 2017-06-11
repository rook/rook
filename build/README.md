# Building Rook

Rook is a golang project and can be built directly on the host with just `make`
and `golang` installed.

Rook can also be built inside a Docker container and results in a consistent
build, test and release environment. We recommend the cross build container, and
our Jenkins build and test environment uses it.

## Building Rook for the host platform

Now you can build in the container normally, like so:

```
make -j4
```

This will build just the binaries and copy them to the applicable subfolder of ./bin.

## Using the Build Container

To run a command inside the container:

```
> build/run <command>
```

This will run `<command>` from inside the container where all tools and dependencies
have been installed and configured. The current directory is set to the source directory
which is bind mounted (or rsync'd on some platforms) inside the container.

The first run of `build/run` will build the container itself and could take a few
minutes to complete.

If you don't pass a command you get an interactive shell inside the container.

```
> build/run
rook@moby:~/go/src/github.com/rook/rook$
```

## Building Rook for all platforms

Now you can build in the container normally, like so:

```
make -j4 build.all
```

## Building all releasable artifacts

To build all release artifats run:

```
make -j4 release
```

## Resetting the build container

If you're running the build container on the Mac using Docker for Mac, the build
container will rely on rsync to copy source to and from the host. To reset the build container and it's persistent volumes, you can run the below command. You shouldn't have to do this often unless something is broken or stale with your build container:

```
build/reset
```
