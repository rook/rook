<img alt="Rook" src="Documentation/media/logo.svg" width="50%" height="50%">

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)
[![GitHub release](https://img.shields.io/github/release/rook/rook/all.svg?style=flat-square)](https://github.com/rook/rook/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/rook/rook.svg)](https://img.shields.io/docker/pulls/rook/rook.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/rook/rook)](https://goreportcard.com/report/github.com/rook/rook)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1599/badge)](https://bestpractices.coreinfrastructure.org/projects/1599)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_shield)
[![Slack](https://slack.rook.io/badge.svg)](https://slack.rook.io)
[![Twitter Follow](https://img.shields.io/twitter/follow/rook_io.svg?style=social&label=Follow)](https://twitter.com/intent/follow?screen_name=rook_io&user_id=788180534543339520)

# What is Rook?

Rook is an open source **cloud-native storage orchestrator** for Kubernetes, providing the platform, framework, and support for a diverse set of storage solutions to natively integrate with cloud-native environments.

Rook turns storage software into self-managing, self-scaling, and self-healing storage services. It does this by automating deployment, bootstrapping, configuration, provisioning, scaling, upgrading, migration, disaster recovery, monitoring, and resource management. Rook uses the facilities provided by the underlying cloud-native container management, scheduling and orchestration platform to perform its duties.

Rook integrates deeply into cloud native environments leveraging extension points and providing a seamless experience for scheduling, lifecycle management, resource management, security, monitoring, and user experience.

For more details about the storage solutions currently supported by Rook, please refer to the [project status section](#project-status) below.
We plan to continue adding support for other storage systems and environments based on community demand and engagement in future releases. See our [roadmap](ROADMAP.md) for more details.

Rook is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF) as an incubating level project. If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Rook plays a role, read the CNCF [announcement](https://www.cncf.io/blog/2018/01/29/cncf-host-rook-project-cloud-native-storage-capabilities).

## Getting Started and Documentation

For installation, deployment, and administration, see our [Documentation](https://rook.github.io/docs/rook/master).

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please open an [issue](https://github.com/rook/rook/issues).

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://slack.rook.io)
- Forums: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Twitter: [@rook_io](https://twitter.com/rook_io)
- Email: [info@rook.io](mailto:info@rook.io)

### Community Meeting

A regular community meeting takes place every other [Tuesday at 9:00 AM PT (Pacific Time)](https://zoom.us/j/392602367).
Convert to your [local timezone](http://www.thetimezoneconverter.com/?t=9:00&tz=PT%20%28Pacific%20Time%29).

Any changes to the meeting schedule will be added to the [agenda doc](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing) and posted to [Slack #announcements](https://rook-io.slack.com/messages/C76LLCEE7/) and the [rook-dev mailing list](https://groups.google.com/forum/#!forum/rook-dev).

Anyone who wants to discuss the direction of the project, design and implementation reviews, or general questions with the broader community is welcome and encouraged to join.

- Meeting link: <https://zoom.us/j/392602367>
- [Current agenda and past meeting notes](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing)
- [Past meeting recordings](https://www.youtube.com/playlist?list=PLP0uDo-ZFnQP6NAgJWAtR9jaRcgqyQKVy)

## Project Status

The status of each storage provider supported by Rook can be found in the table below.
Each API group is assigned its own individual status to reflect their varying maturity and stability.
More details about API versioning and status in Kubernetes can be found on the Kubernetes [API versioning page](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning), but the key difference between the statuses are summarized below:

- **Alpha:** The API may change in incompatible ways in a later software release without notice, recommended for use only in short-lived testing clusters, due to increased risk of bugs and lack of long-term support.
- **Beta:** Support for the overall features will not be dropped, though details may change. Support for upgrading or migrating between versions will be provided, either through automation or manual steps.
- **Stable:** Features will appear in released software for many subsequent versions and support for upgrading between versions will be provided with software automation in the vast majority of scenarios.

| Name           | Details                                                                                                                                                                                                                                                                                                                | API Group                    | Status |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- | ------ |
| Rook Framework | The framework for common storage specs and logic used to support other storage providers.                                                                                                                                                                                                                              | rook.io/v1alpha2             | Alpha  |
| Ceph           | [Ceph](https://ceph.com/) is a distributed storage system that provides file, block and object storage and is deployed in large scale production clusters.                                                                                                                                                             | ceph.rook.io/v1              | Stable |
| CockroachDB    | [CockroachDB](https://www.cockroachlabs.com/product/cockroachdb/) is a cloud-native SQL database for building global, scalable cloud services that survive disasters.                                                                                                                                                  | cockroachdb.rook.io/v1alpha1 | Alpha  |
| Cassandra      | [Cassandra](http://cassandra.apache.org/) is a highly available NoSQL database featuring lightning fast performance, tunable consistency and massive scalability. [Scylla](https://www.scylladb.com) is a close-to-the-hardware rewrite of Cassandra in C++, which enables much lower latencies and higher throughput. | cassandra.rook.io/v1alpha1   | Alpha  |
| EdgeFS         | [EdgeFS](http://edgefs.io) is high-performance and fault-tolerant decentralized data fabric with access to object, file, NoSQL and block.                                                                                                                                                                              | edgefs.rook.io/v1            | Stable |
| NFS            | [Network File System (NFS)](https://github.com/nfs-ganesha/nfs-ganesha/wiki) allows remote hosts to mount file systems over a network and interact with those file systems as though they are mounted locally.                                                                                                         | nfs.rook.io/v1alpha1         | Alpha  |
| YugabyteDB     | [YugabyteDB](https://docs.yugabyte.com/latest/introduction/) is a high-performance, cloud-native distributed SQL database which can tolerate disk, node, zone and region failures automatically.                                                                                                                       | yugabytedb.rook.io/v1alpha1  | Alpha  |

### Official Releases

Official releases of Rook can be found on the [releases page](https://github.com/rook/rook/releases).
Please note that it is **strongly recommended** that you use [official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the master branch are subject to changes and incompatibilities that will not be supported in the official releases.
Builds from the master branch can have functionality changed and even removed at any time without compatibility support and without prior notice.

## Licensing

Rook is under the Apache 2.0 license.

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_large)
