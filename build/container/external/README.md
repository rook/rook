# External Packages

Rook relies on a number of external packages. These are all built from source
and packaged into the build container.

To build the packages directly run the following on a modern linux distro:

```
./build.sh
```

You can also build the pacakges inside the build container:

To build the packages directly run the following:

```
build/container/external/build.sh
```

To run a prallel build run the following:

```
./build.sh -j4
```

To build only a specific package specify the pacakge name as follows:

```
./build.sh rocksdb
```
