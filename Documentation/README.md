# Rook

Rook is an open source **cloud-native storage orchestrator**, providing the platform, framework, and support for a diverse set of storage solutions to natively integrate with cloud-native environments.

Rook turns storage software into self-managing, self-scaling, and self-healing storage services. It does this by automating deployment, bootstrapping, configuration, provisioning, scaling, upgrading, migration, disaster recovery, monitoring, and resource management. Rook uses the facilities provided by the underlying cloud-native container management, scheduling and orchestration platform to perform its duties.

Rook integrates deeply into cloud native environments leveraging extension points and providing a seamless experience for scheduling, lifecycle management, resource management, security, monitoring, and user experience.

For more details about the status of storage solutions currently supported by Rook, please refer to the [project status section](https://github.com/rook/rook/blob/master/README.md#project-status) of the Rook repository.
We plan to continue adding support for other storage systems and environments based on community demand and engagement in future releases.

## Quick Start Guides

Starting Rook in your cluster is as simple as a few `kubectl` commands depending on the storage provider.
See our [Quickstart](quickstart.md) guide list for the detailed instructions for each storage provider.

## Storage Provider Designs

High-level Storage Provider design documents:

| Storage Provider            | Status | Description                                                                                                                                            |
| --------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [Ceph](ceph-storage.md)     | Stable | Ceph is a highly scalable distributed storage solution for block storage, object storage, and shared filesystems with years of production deployments. |
| [EdgeFS](edgefs-storage.md) | Stable | EdgeFS is high-performance and fault-tolerant decentralized data fabric with access to object, file, NoSQL and block.                         |

Low level design documentation for supported list of storage systems collected at [design docs](https://github.com/rook/rook/tree/master/design) section.

## Need help? Be sure to join the Rook Slack

If you have any questions along the way, please don't hesitate to ask us in our [Slack channel](https://rook-io.slack.com). You can sign up for our Slack [here](https://slack.rook.io).
