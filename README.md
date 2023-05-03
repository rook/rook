<img alt="Rook" src="Documentation/media/logo.svg" width="50%" height="50%">

[![CNCF Status](https://img.shields.io/badge/cncf%20status-graduated-blue.svg)](https://www.cncf.io/projects)
[![GitHub release](https://img.shields.io/github/release/rook/rook/all.svg)](https://github.com/rook/rook/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/rook/ceph)](https://hub.docker.com/u/rook)
[![Go Report Card](https://goreportcard.com/badge/github.com/rook/rook)](https://goreportcard.com/report/github.com/rook/rook)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1599/badge)](https://bestpractices.coreinfrastructure.org/projects/1599)
[![Security scanning](https://github.com/rook/rook/actions/workflows/synk.yaml/badge.svg)](https://github.com/rook/rook/actions/workflows/synk.yaml)
[![Slack](https://img.shields.io/badge/rook-slack-blue)](https://slack.rook.io)
[![Twitter Follow](https://img.shields.io/twitter/follow/rook_io.svg?style=social&label=Follow)](https://twitter.com/intent/follow?screen_name=rook_io&user_id=788180534543339520)

# What is Rook?

Rook is an open source **cloud-native storage orchestrator** for Kubernetes, providing the platform, framework, and support for Ceph storage to natively integrate with Kubernetes.

[Ceph](https://ceph.com/) is a distributed storage system that provides file, block and object storage and is deployed in large scale production clusters.

Rook automates deployment and management of Ceph to provide self-managing, self-scaling, and self-healing storage services.
The Rook operator does this by building on Kubernetes resources to deploy, configure, provision, scale, upgrade, and monitor Ceph.

The status of the Ceph storage provider is **Stable**. Features and improvements will be planned for many future versions. Upgrades between versions are provided to ensure backward compatibility between releases.

Rook is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF) as a [graduated](https://www.cncf.io/announcements/2020/10/07/cloud-native-computing-foundation-announces-rook-graduation/) level project. If you are a company that wants to help shape the evolution of technologies that are container-packaged, dynamically-scheduled and microservices-oriented, consider joining the CNCF. For details about who's involved and how Rook plays a role, read the CNCF [announcement](https://www.cncf.io/blog/2018/01/29/cncf-host-rook-project-cloud-native-storage-capabilities).

## Getting Started and Documentation

For installation, deployment, and administration, see our [Documentation](https://rook.github.io/docs/rook/latest-release) and [QuickStart Guide](https://rook.github.io/docs/rook/latest-release/Getting-Started/quickstart).

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please open an [issue](https://github.com/rook/rook/issues).

### Reporting Security Vulnerabilities

If you find a vulnerability or a potential vulnerability in Rook please let us know immediately at
[cncf-rook-security@lists.cncf.io](mailto:cncf-rook-security@lists.cncf.io). We'll send a confirmation email to acknowledge your
report, and we'll send an additional email when we've identified the issues positively or
negatively.

For further details, please see the complete [security release process](SECURITY.md).

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://slack.rook.io)
- GitHub: Start a [discussion](https://github.com/rook/rook/discussions) or open an [issue](https://github.com/rook/rook/issues)
- Twitter: [@rook_io](https://twitter.com/rook_io)
- Security topics: [cncf-rook-security@lists.cncf.io](#reporting-security-vulnerabilities)

## Community Meeting

A regular community meeting takes place every other [Tuesday at 9:00 AM PT (Pacific Time)](https://zoom.us/j/392602367?pwd=NU1laFZhTWF4MFd6cnRoYzVwbUlSUT09).
Convert to your [local timezone](http://www.thetimezoneconverter.com/?t=9:00&tz=PT%20%28Pacific%20Time%29).

Any changes to the meeting schedule will be added to the [agenda doc](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing) and posted to [Slack #announcements](https://rook-io.slack.com/messages/C76LLCEE7/).

Anyone who wants to discuss the direction of the project, design and implementation reviews, or general questions with the broader community is welcome and encouraged to join.

- Meeting link: <https://zoom.us/j/392602367?pwd=NU1laFZhTWF4MFd6cnRoYzVwbUlSUT09>
- [Current agenda and past meeting notes](https://docs.google.com/document/d/1exd8_IG6DkdvyA0eiTtL2z5K2Ra-y68VByUUgwP7I9A/edit?usp=sharing)
- [Past meeting recordings](https://www.youtube.com/playlist?list=PLP0uDo-ZFnQP6NAgJWAtR9jaRcgqyQKVy)

## Official Releases

Official releases of Rook can be found on the [releases page](https://github.com/rook/rook/releases).
Please note that it is **strongly recommended** that you use [official releases](https://github.com/rook/rook/releases) of Rook, as unreleased versions from the master branch are subject to changes and incompatibilities that will not be supported in the official releases.
Builds from the master branch can have functionality changed and even removed at any time without compatibility support and without prior notice.

## Licensing

Rook is under the Apache 2.0 license.

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_large)
