---
title: Running Rook in AWS
weight: 25
indent: true
---

# Running Rook in AWS

Rook can functionally replace Elastic Block Storage (EBS), Elastic File System (EFS) and S3 with Rook ObjectStore. Potentially reducing storage cost, increasing performance and portability of your Kubernetes workloads.

:information_source:  
Please refer to [Creating Rook Clusters][3] on how to customize your cluster to use specific devices.

* To deliver robust performance at reasonable cost, consider using instances backed by [*instance store*][1] for Rook cluster nodes. Utilization of storage devices directly attached to the instance will reduce latency and provide more consistent IO performance.
* The ephemeral nature of the *instance store* is handled by resiliency build into Rook. Make sure to configure `replicas` parameter in accordance with your durability requirements.
* Multiple EBS devices can be [striped into RAID array][2] and used as Rook devices to improve performance of low cost EBS volumes.
* Rook cluster nodes can be dedicated to manage your storage, or you can choose to utilize compute resources of Rook nodes to run Kubernetes workloads with proper resource isolation, father reducing your cluster costs.

## Benefits of using Rook instead of EC2 storage services (EBS, EFS, S3)

* Portability. Avoiding vendor lock-in, supporting multi-platform deployments and manage your storage in a Cloud Native way.
* Performance. Significantly higher IO performance can be achieved at lower cost.
* Improvement in storage provisioning times compared to EC2 storage services.
* Using Rook ObjectStore instead of S3 provides more flexibility in terms of customization of end points and authentication not tied to a specific provider.

[1]: http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/InstanceStorage.html
[2]: http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/raid-config.html
[3]: ./cluster-crd.md