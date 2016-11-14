![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/job/ci-rook/badge/icon)](https://jenkins.rook.io/job/ci-rook/)

## Open, Cloud Native, and Universal Distributed Storage

- [What is Rook?](#what-is-rook)
- [Status](#status)
- [Quickstart](#quickstart)
- [Building](#building)
- [Design](#design)
- [Contributing](#contributing)
- [Contact](#contact)
- [Licensing](#licensing)

## What is Rook?

Rook is a distributed storage system designed for cloud native applications. It
exposes file, block, and object storage on top of shared resource pools. Rook has minimal
dependencies and can be deployed in dedicated storage clusters or converged clusters. It's
self-managing, self-protecting, self-healing, and is designed to just work without teams of
engineers managing it. It scales from a single node, to multi-PB clusters spread geographically.
It's based on the Ceph project with over 10 years of production deployments in some of the
largest storage clusters in the world.

## Status

Rook is in **alpha** state. We're just getting started. Not all planned features are complete. The API
and other user-facing objects are subject to change. Backward-compability is not supported for this
release. See our [Roadmap](https://github.com/rook/rook/wiki/Roadmap) and [Issues](https://github.com/rook/rook/issues).
Please help us by [Contributing](CONTRIBUTING.md) to the project.

## Quickstart

Here's the quickest way to get going with Rook.

### Linux

On a modern Linux host run the following:

1. Download the latest  binaries

    ```bash
    $ wget https://github.com/rook/rook/releases/download/v0.1.0/rook-v0.1.0-linux-amd64.tar.gz
    $ tar xvf rook-v0.1.0-linux-amd64.tar.gz
    ```

2. Start a one node Rook cluster

    ```bash
    $ ./rookd --data-dir /tmp/rook-test
    ```

3. Now in a different shell (in the same path) create a new volume image (10MB)

    ```bash
    $ ./rook block create --name test --size 10485760
    ```

4. Mount the block volume and format it

    ```bash
    sudo ./rook block mount --name test --path /tmp/rook-volume
    sudo chown $USER:$USER /tmp/rook-volume
    ```

5. Write and read a file

    ```bash
    echo "Hello Rook!" > /tmp/rook-volume/hello
    cat /tmp/rook-volume/hello
    ```

6. Cleanup

    ```bash
    sudo ./rook block unmount --path /tmp/rook-volume
    ```

### Kubernetes

To run a Kubernetes cluster with Rook for persistent storage go [here](https://github.com/rook/coreos-kubernetes)

### CoreOS

Rook is also easy to run on CoreOS either directly on the host or via rkt.

```
cd demo/vagrant
vagrant up
```

## Building

See [Building](https://github.com/rook/rook/wiki/Building) in the wiki for more details.

## Design

A rook cluster is made up of one or more nodes each running the Rook daemon `rookd`. Containers and Pods can
mount block devices and filesystems exposed by the cluster, or can use S3/Swift API for object storage. There is
also a REST API exposed by `rookd` as well as a command line tool called `rook`.

![Overview](Documentation/media/cluster.png)

The Rook daemon `rookd` is a single binary that is self-contained and has all that is needed to bootstrap, scale
and manage a storage cluster. `rookd` is typically compiled into a single static binary (just like most golang
binaries) or a dynamic binary that takes a dependency on mostly libc. It can run in minimal containers, alongside a
hypervisor, or directly on the host on most Linux distributions.

`rookd` uses an embedded version of Ceph for storing all data -- there are no changes to the data path. An embedded version
of Ceph was created specifically for Rook scenarios and has been pushed upstream. Rook does not attempt to maintain full fidelity
with Ceph, for example, most of the Ceph concepts like OSDs, MONs, placement groups, etc. are hidden. Instead Rook creates
a much simplified UX for admins that is in terms of physical resources, pools, volumes, filesystems, and buckets.

`rookd` embeds Etcd to store configuration and coordinate cluster-wide management operations. `rookd` will automatically
bootstrap Etcd, manage it, and scale it as the cluster grows. It's also possible to use an external Etcd instead of the embedded one
if needed.

Rook and etcd are implemented in golang. Ceph is implemented in C++ where the data path is highly optimized. We believe
this combination offers the best of both worlds.

See [Design](https://github.com/rook/rook/wiki/Design) wiki for more details.

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, help us out by opening an [issue](https://github.com/rook/rook/issues).

## Contact

Please use the following to reach members of the community:

- Email: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Gitter: [rook-dev](https://gitter.im/rook/rook-dev)

## Licensing

Rook and Etcd are under the Apache 2.0 license. Ceph is mostly under the LGPL 2.0 license. Some portions
of the code are under different licenses. The appropriate license information can be found in the headers
of the source files.
