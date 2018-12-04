# Legacy Block Storage with Rook


## Overview

### Value of legacy block storage support to Rook

* In order to completely integrate with Kubernetes and utilize Rook's autonomous healing, scaling, and managing features, backend storage systems need to be software-based, distributed (clustered), and capable of being containerized. 
Not all modern storage systems meet these requirements.
* Many expensive and proprietary appliance storage silos are still providing services for Enterprise IT infrastructures.
* Many companies only trust well-certified and time-proven storage solutions for their mission critical applications

For transitioning into existing enterprise IT infrastructures, Rook will have a much easier time with support for legacy block storage systems.


### Storage Resource Normalization

The OpenStack Block Storage service (Cinder) provides a standardized architecture to aggregate the features and management of all kinds of block storage systems.
It is the first service to have a standardized interface that is widely supported by most storage vendors including Dell EMC, IBM, NetApp, Hitachi, HPE, Pure Storage, VMware, etc.

The major functions of the Cinder volume driver include:

* Create/Delete Volume
* Attach/Detach Volume
* Extend Volume
* Create/Delete Snapshot
* Create Volume from Snapshot
* Create Volume from Volume (clone)
* Create Image from Volume
* Volume Migration (host assisted)

The following are some references for OpenStack Cinder:

* [OpenStack Cinder](https://access.redhat.com/documentation/en/red-hat-enterprise-linux-openstack-platform/version-7/red-hat-enterprise-linux-openstack-platform-7-architecture-guide/chapter-1-components#comp-cinder)
* [Cinder Driver Support Matrix](https://wiki.openstack.org/wiki/CinderSupportMatrix)

In this project, we propose to leverage the open source project [OpenStack Cinder](https://github.com/openstack/cinder) as the unified storage connector that integrates legacy block storage with Rook.
Through this implementation, we can quickly extend Rook's storage backend support to most modern block storage solutions that are already used in production today.


## Architecture

![legacy-block-storage-architecture-diagram](legacy-block-storage-architecture.png)


