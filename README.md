![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)

## Open, Cloud Native, and Universal Distributed Storage

- [What is Rook?](#what-is-rook)
- [Status](#status)
- [Quickstart Guides](#quickstart-guides)
- [Advanced Configuration and Troubleshooting](#advanced-configuration-and-troubleshooting)
- [Building](#building)
- [Contributing](#contributing)
- [Contact](#contact)
- [Licensing](#licensing)

## What is Rook?

Rook is a distributed storage system designed for cloud native applications. It exposes file, block, and object storage on top of shared resource pools.
Rook has minimal dependencies and can be deployed in dedicated storage clusters or converged clusters.
It's self-managing, self-protecting, self-healing, and is designed to just work without teams of engineers managing it.
It scales from a single node, to multi-PB clusters spread geographically.

It is based on the [Ceph](http://ceph.com) project that has over 10 years of production deployments in some of the largest storage clusters in the world.

Rook integrates deeply into popular container environments like Kubernetes and leverages facilities for lifecycle management, resource management, scale-out and upgrades.
Rook also integrates into the Kubernetes API to expose a uniform surface area for management.

## Status

Rook is in **alpha** state. We're just getting started. Not all planned features are complete. The API
and other user-facing objects are subject to change. Backward-compability is not supported for this
release. See our [Roadmap](https://github.com/rook/rook/wiki/Roadmap) and [Issues](https://github.com/rook/rook/issues).
Please help us by [Contributing](CONTRIBUTING.md) to the project.

## Quickstart Guides

There are a few different options for running a Rook cluster for your storage needs.  Kubernetes is the recommended way because of the rich orchestration and scheduling that Kubernetes provides via the Rook operator.

1. [Kubernetes](Documentation/kubernetes.md) (recommended)
2. [Standalone](Documentation/standalone.md)

### Using Rook

Once you have a Rook cluster running, you can use the `rookctl` tool to create and manage storage as shown in the following guide:
- [Using Rook Guide](Documentation/client.md)

## Advanced Configuration and Troubleshooting

Our Rook toolbox container is available to aid with troubleshooting and advanced configuration of your Rook cluster.
It automatically configures a Ceph client suite to work with your Rook deployment, and additional tools are just an `apt-get` away.

To get started please see the [toolbox readme](Documentation/toolbox.md).  Also see our [advanced configuration](Documentation/advanced-configuration.md) document for helpful maintenance and tuning examples.

## Building

See [Building](https://github.com/rook/rook/wiki/Building) in the wiki for more details.

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, help us out by opening an [issue](https://github.com/rook/rook/issues).

## Contact

Please use the following to reach members of the community:

- Email: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Gitter: [rook/rook](https://gitter.im/rook/rook) for general project discussions or [rook-dev](https://gitter.im/rook/rook-dev) for development discussions.
- Twitter: [@rook_io](https://twitter.com/rook_io)

## Licensing

Rook and Etcd are under the Apache 2.0 license. [Ceph](https://github.com/rook/ceph/blob/master/COPYING) is mostly under the LGPL 2.0 license. Some portions
of the code are under different licenses. The appropriate license information can be found in the headers
of the source files.
