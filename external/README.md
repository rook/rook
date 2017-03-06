# Rook External Dependencies

Rook relies on Ceph and a number of other packages that are written in C/C++.
This directory builds all of these packages from source so that they can be
linked into the Rook binary.

On a modern linux distro, run the following to build the packages:

```
make
```

To run a prallel build run the following:

```
make -j4
```

To see verbose logging run:

```
make V=1
```

To build for all supported platforms:

```
make cross
```

ccache can significantly speed up the build. Its turned on by default. To disable:

```
make CCACHE=0
```

You can also build the packages inside our cross build container:

```
build/run make -C external
```

See `make help` for more options.