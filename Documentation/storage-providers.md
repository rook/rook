---
title: Storage Providers
weight: 12050
indent: true
---

# Storage Providers

Rook is the home for operators for multiple storage providers. Each of these storage providers
has specific requirements and each of them is very independent. There is no runtime dependency
between the storage providers. Development is where the storage providers benefit from one another.

Rook provides a development framework with a goal of enabling storage providers to create
operators for Kubernetes to manage their storage layer. As the storage provider community
grows, we expect this framework to grow as common storage constructs are identified
that will benefit the community. Rook does not aim to replace other frameworks or
communities, but to fill gaps not provided by other core projects.

Storage providers in Rook are currently built on the [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime),
but may also be built on other frameworks such as the [Operator SDK](https://sdk.operatorframework.io/)
or [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder). The choice of the
underlying framework is up to the storage provider.

Rook does not aim to be a general framework for storage, but to provide a
very specific set of helpers to meet the storage provider needs in the Rook project.

## Rook Framework

Rook provides the following framework to assist storage providers in building an operator:

* Common golang packages shared by storage providers are in the main [Rook repo](https://github.com/rook/rook).
* Common build scripts for building the operator images are in the main
  [Rook repo](https://github.com/rook/rook/tree/master/build).
* Each provider has its own repo under the [Rook org](https://github.com/rook).
  * Multiple community members are given push access to the repo, including
    owners of the storage provider, Rook steering committee members,
    and other Rook maintainers if deemed helpful or necessary by the steering
    committee. Maintainers for the new provider are added according to the
    [governance](https://github.com/rook/rook/blob/master/GOVERNANCE.md).
  * Providers added to Rook prior to 2020 are grandfathered into the main
    [Rook repo](https://github.com/rook/rook).
* Storage providers must follow the Rook [governance](https://github.com/rook/rook/blob/master/GOVERNANCE.md)
  in the interest of the good of the overall project. Storage providers have
  autonomy in their feature work, while collaboration with the community
  is expected for shared features.
* A quarterly release cadence is in place for the operators in the main Rook repo.
  Operators in their own repo define their own cadence and versioning scheme as desired.
  * Storage providers own their release process, while following Rook best practices to
    ensure high quality.
  * Each provider owns independent CI based on GitHub actions, with patterns and build
    automation that can be re-used by providers
* Docker images are pushed to the [Rook DockerHub](https://hub.docker.com/u/rook) where
  each storage provider has its own repo.
* Helm charts are published to [charts.rook.io](https://charts.rook.io/release)
* Documentation for the storage provider is to be written by the storage provider
  members. The build process will publish the documentation to the [Rook website](https://rook.github.io/docs/rook/latest/).
* All storage providers are added to the [Rook.io website](https://rook.io/)
* A great Slack community is available where you can communicate amongst developers and users

## Considering Joining Rook?

If you own a storage provider and are interested in joining the Rook project to create
an operator, please consider the following:

* You are making a clear commitment to the development of the storage provider.
  Creating an operator is not a one-time engineering cost, but is a long term commitment
  to the community.
* Support for a storage provider in Rook requires dedication and community support.
* Do you really need an operator? Many storage applications (e.g. CSI drivers)
  can be deployed with tools such as a Helm chart and don't really need the
  flexibility of an operator.
* Joining Rook is also about community, not just the framework.

## Engineering Requirements

The engineering costs of each storage provider include:

* Develop the operator
* Rook maintainers will help answer questions along the way, but ultimately
  you own the development
* If there are test failures in the CI, they should be investigated in a timely manner
* If issues are opened in GitHub, they need investigation and triage to provide
  expectations about the priority and timeline
* If users have questions in Slack, they should be answered in a timely manner.
  Community members can also be redirected to other locations if desired for the provider.
* A regular cadence of releases is expected. Software always needs to evolve with new versions
  of K8s, accommodate new features in the storage provider, etc.
* Each provider maintains a ROADMAP.md in the root of their repo, updates it regularly
  (e.g. quarterly or with the release cadence), and provides input to the overall Rook
  [roadmap](https://github.com/rook/rook/blob/master/ROADMAP.md) for common features.

### Inactive Providers

If a storage provider does not have engineering resources, Rook cannot claim to support it.
After some months of inactivity Rook will deprecate a storage provider. The timing
will be decided on a case by case basis by the steering committee. The repo and other artifacts
for deprecated storage providers will be left intact for reference.
