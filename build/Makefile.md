## Link Mode

`rookd` is golang binary that uses cgo to interface with embedded Ceph. The Ceph
project is a submodule of Rook and is built from the top level Makefile. Ceph
has numerous dependencies. We support the following link modes when linking:

### All Static

This option produces completely static binaries for `rookd` and `rook`. These
binaries can easily be used inside scratch containers, or on distributions
where dependencies can not be easily satisfied. To build statically we recommend
the you build only through the build container. While its possible to build statically
on the host, you might not be able to satisfy the needed dependencies.

```
build/run make LINKMODE=static
```

Here's an example of what the dependencies look like in this mode:

```
> ldd bin/linux_amd64/rookd
	not a dynamic executable
```

### All Dynamic (Default)

This option produces binaries that link dynamically to dependent packages, and
is intended for traditional Linux distributions like Ubuntu or CentOS. All
dependencies would need to be installed on the system for `rookd` to run
correctly. This is the default LINKMODE.

```
make LINKMODE=dynamic
```

In this mode the system dependencies can vary from one distro to another. Here's an
example of what the dependencies look like on Ubuntu:

```
> build/run ldd bin/linux_amd64/rookd
	linux-vdso.so.1 =>  (0x00007ffc053c3000)
	libboost_system.so.1.62.0 => /usr/local/lib/x86_64-linux-gnu/libboost_system.so.1.62.0 (0x00007f56f6e33000)
	libboost_thread.so.1.62.0 => /usr/local/lib/x86_64-linux-gnu/libboost_thread.so.1.62.0 (0x00007f56f6c0b000)
	libboost_iostreams.so.1.62.0 => /usr/local/lib/x86_64-linux-gnu/libboost_iostreams.so.1.62.0 (0x00007f56f69f6000)
	libboost_random.so.1.62.0 => /usr/local/lib/x86_64-linux-gnu/libboost_random.so.1.62.0 (0x00007f56f67f0000)
	libblkid.so.1 => /usr/local/lib/x86_64-linux-gnu/libblkid.so.1 (0x00007f56f65aa000)
	libz.so.1 => /usr/local/lib/x86_64-linux-gnu/libz.so.1 (0x00007f56f6392000)
	libsnappy.so.1 => /usr/lib/x86_64-linux-gnu/libsnappy.so.1 (0x00007f56f6184000)
	libcrypto++.so.6 => /usr/lib/x86_64-linux-gnu/libcrypto++.so.6 (0x00007f56f5c05000)
	libaio.so.1 => /lib/x86_64-linux-gnu/libaio.so.1 (0x00007f56f5a03000)
	libcurl.so.4 => /usr/local/lib/x86_64-linux-gnu/libcurl.so.4 (0x00007f56f57cd000)
	libexpat.so.1 => /lib/x86_64-linux-gnu/libexpat.so.1 (0x00007f56f5396000)
	libdl.so.2 => /lib/x86_64-linux-gnu/libdl.so.2 (0x00007f56f5192000)
	libresolv.so.2 => /lib/x86_64-linux-gnu/libresolv.so.2 (0x00007f56f4f77000)
	libpthread.so.0 => /lib/x86_64-linux-gnu/libpthread.so.0 (0x00007f56f4d59000)
	libstdc++.so.6 => /usr/lib/x86_64-linux-gnu/libstdc++.so.6 (0x00007f56f49d1000)
	libm.so.6 => /lib/x86_64-linux-gnu/libm.so.6 (0x00007f56f46c8000)
	libgcc_s.so.1 => /lib/x86_64-linux-gnu/libgcc_s.so.1 (0x00007f56f44af000)
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f56f40e8000)
	/lib64/ld-linux-x86-64.so.2 (0x0000564ace68d000)
	librt.so.1 => /lib/x86_64-linux-gnu/librt.so.1 (0x00007f56f3ee0000)
	libuuid.so.1 => /usr/local/lib/x86_64-linux-gnu/libuuid.so.1 (0x00007f56f3cdb000)
	(more)
```

Note the Libraries like libcurl can bring in lots of other dependencies, and vary widely
between distributions.

### Dynamic Standard Library

This option produces mostly static binaries, whith just the standard C/C++ libraries
dynamically linked. Its intended for restricted Linux environments like CoreOS when
running directly on the host OS (and not inside a container).

```
make LINKMODE=stdlib
```

Here's an example of what the dependencies look like in this mode:

```
> ldd bin/linux_amd64/rookd
	linux-vdso.so.1 =>  (0x00007fff45fb6000)
	libdl.so.2 => /lib/x86_64-linux-gnu/libdl.so.2 (0x00007f5305d4e000)
	libresolv.so.2 => /lib/x86_64-linux-gnu/libresolv.so.2 (0x00007f5305b33000)
	libpthread.so.0 => /lib/x86_64-linux-gnu/libpthread.so.0 (0x00007f5305915000)
	libstdc++.so.6 => /usr/lib/x86_64-linux-gnu/libstdc++.so.6 (0x00007f530558d000)
	libm.so.6 => /lib/x86_64-linux-gnu/libm.so.6 (0x00007f5305284000)
	libgcc_s.so.1 => /lib/x86_64-linux-gnu/libgcc_s.so.1 (0x00007f530506b000)
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f5304ca4000)
	/lib64/ld-linux-x86-64.so.2 (0x000055d3eb36e000)
```

## Hardened binary (PIE)

You can build a Position Independent executable. Note that this requires LINKMODE set
to `dynamic` or `stdlib`

```
make LINKMODE=stdlib PIE=1
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

C++ code can take time to compile. CCache can increase the speed of compilation. If you're
using the build container this is enabled by default. To enable make sure you have
ccache installed:

```
apt-get install ccache
```

to run with ccache enabled run the following:

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

## Cross Compiling

To cross compile run:

```
make -j4 cross
```

This will build Rook for all supported platforms. If you want to cross compile for a specific
platform pass GOOS and GOARCH as follows:

```
make -j4 GOOS=windows GOARCH=amd64
```

Note that `rookd` is only supported on Linux and it will be skipped when building on an
unsupported OS or arch.

