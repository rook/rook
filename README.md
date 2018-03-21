![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)
[![GitHub release](https://img.shields.io/github/release/rook/rook/all.svg?style=flat-square)](https://github.com/rook/rook/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/rook/rook)](https://goreportcard.com/report/github.com/rook/rook)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_shield)
[![Slack](https://rook-slackin.herokuapp.com/badge.svg)](https://rook-slackin.herokuapp.com/badge.svg)
[![Twitter Follow](https://img.shields.io/twitter/follow/rook_io.svg?style=social&label=Follow)](https://twitter.com/intent/follow?screen_name=rook_io&user_id=788180534543339520)

## What is Rook?

Rook is an open source orchestrator for distributed storage systems running in cloud native environments.

Rook turns distributed storage software into a self-managing, self-scaling, and self-healing storage services. It does this by automating deployment, bootstrapping, configuration, provisioning, scaling, upgrading, migration, disaster recovery, monitoring, and resource management. Rook uses the facilities provided by the underlying cloud-native container management, scheduling and orchestration platform to perform its duties.

Rook integrates deeply into cloud native environments leveraging extension points and providing a seamless experience for scheduling, lifecycle management, resource management, security, monitoring, and user experience.

Rook is currently in alpha state and has focused initially on orchestrating Ceph on-top of Kubernetes. Ceph is a distributed storage system that provides file, block and object storage and is deployed in large scale production clusters. Rook plans to add support for other storage systems beyond Ceph and other cloud native environments beyond Kubernetes in future releases. See our [roadmap](ROADMAP.md) for more details.

Rook is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF) as an inception level project. If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Rook plays a role, read the CNCF [announcement](https://www.cncf.io/blog/2018/01/29/cncf-host-rook-project-cloud-native-storage-capabilities).

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

### Community Meeting
A regular community meeting takes place every other [Tuesday at 9:00 AM PT (Pacific Time)](https://zoom.us/j/392602367).
Convert to your [local timezone](http://www.thetimezoneconverter.com/?t=9:00&tz=PT%20%28Pacific%20Time%29).

Any changes to the meeting schedule will be added to the [agenda doc](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing) and posted to [Slack #announcements](https://rook-io.slack.com/messages/C76LLCEE7/) and the [rook-dev mailing list](https://groups.google.com/forum/#!forum/rook-dev).

Anyone who wants to discuss the direction of the project, design and implementation reviews, or general questions with the broader community is welcome and encouraged to join.
* Meeting link: https://zoom.us/j/392602367
* [Current agenda and past meeting notes](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing)
* [Past meeting recordings](https://www.youtube.com/playlist?list=PLP0uDo-ZFnQP6NAgJWAtR9jaRcgqyQKVy)

## Licensing

Rook is under the Apache 2.0 license.

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_large)
