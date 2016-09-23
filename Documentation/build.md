# Building

Castle is golang binary that uses cgo to interface with embedded ceph. The ceph 
project is a submodule of castle and is built from the top level Makefile.

## Static binary

This is the easiest option and produced static binaries for castled and tools. These
binaries can easily be used inside minimal or scratch containers, or run on minimal
linux distributions like CoreOS.

```
make STATIC=1
```

## Dyanmic binary

If you dont want to distribute a static binary, a dynamically linked binary is 
supported. The approach we take is to link most of the glibc binaries dynamically
and the rest of the libraries continue to be linked statically. Ceph has a lot
of dependencies and we take this approach to simplify the distribution.

```
make STATIC=0
```

## Hardened binary (PIE)

You can build a Position Independent executable as follows:

```
make STATIC=0 PIE=1
```

## Switching Allocators

Using a different memory allocator can impact the overall performance of the system. 
Three allocators are currently supported: jemalloc, tcmalloc and libc. To specify
an allocator during the build run the following:

```
make ALLOCATOR=jemalloc
```

## Debug Builds

To build a binary with debug symbols run the following:

```
make DEBUG=1
```

Note the binary will be significantly larger in size.

## Verbose Builds

To turn on verbose build output run the following

```
make V=1
```

## Parallel Builds

You can speed up the build significantly by passing the -j flag to make as follows:

```
make -j4
```

## Enabling ccache

C++ code can take time to compile. CCache can increase the speed of compilation. To
enable make sure you have ccache installed:

```
apt-get install ccache
```

to run with ccache enable run the following:

```
export CCACHE_DIR=~/.ccache
make
```

to check ccache stats run:

```
ccache -s
```

to clear the stats run:

```
ccache -z
```

to clear the cache run:

```
ccache -C
```
