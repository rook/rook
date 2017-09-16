![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)
[![GitHub release](https://img.shields.io/github/release/rook/rook/all.svg?style=flat-square)](https://github.com/rook/rook/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/rook/rook)](https://goreportcard.com/report/github.com/rook/rook)
[![Slack](https://rook-slackin.herokuapp.com/badge.svg)](https://rook-slackin.herokuapp.com/badge.svg)
[![Twitter Follow](https://img.shields.io/twitter/follow/rook_io.svg?style=social&label=Follow)](https://twitter.com/intent/follow?screen_name=rook_io&user_id=788180534543339520)

## What is Rook?

Rook is an open source orchestrator for distributed storage systems running in cloud native environments.

Rook turns distributed storage software into a self-managing, self-scaling, and self-healing storage services. It does this by automating deployment, bootstrapping, configuration, provisioning, scaling, upgrading, migration, disaster recovery, monitoring, and resource management. Rook uses the facilities provided by the underlying cloud-native container management, scheduling and orchestration platform to perform its duties.

Rook integrates deeply into cloud native environments leveraging extension points and providing a seamless experience for scheduling, lifecycle management, resource management, security, monitoring, and user experience.

Rook is currently in alpha state and has focused initially on orchestrating Ceph on-top of Kubernetes. Ceph is a distributed storage system that provides file, block and object storage and is deployed in large scale production clusters. Rook plans to add support for other storage systems beyond Ceph and other cloud native environments beyond Kubernetes in future releases. See our [roadmap](ROADMAP.md) for more details.

## Getting Started and Documentation

For installation, deployment, and administration, see our [Documentation](https://rook.github.io/docs/rook/master).

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please open an [issue](https://github.com/rook/rook/issues).

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://rook-slackin.herokuapp.com)
- Forums: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Twitter: [@rook_io](https://twitter.com/rook_io)
- Email: [info@rook.io](mailto:info@rook.io)

## Licensing

Rook is under the Apache 2.0 license.
