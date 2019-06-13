# ♜ Rook NooBaa Design

NooBaa is an object data service for hybrid and multi cloud environments. NooBaa runs on kubernetes, provides an S3 object store service (and Lambda with bucket triggers) to clients both inside and outside the cluster, and uses storage resources from within or outside the cluster, with flexible placement policies to automate data use cases.

[About NooBaa](noobaa-about.md)

# Operator Design

CRDs
- [NooBaaSystem](noobaa-system.md) - The basic CRD to deploy a NooBaa service.
- [NooBaaBackingStore](noobaa-backing-store.md) - Connection to cloud or local storage to use in policies.
- [NooBaaBucketClass](noobaa-bucket-class.md) - Policies applied to a class of buckets.

Applications
- [S3 Account](noobaa-s3-account.md) - Method to obtain S3 credentials for native S3 applications.
- [OBC Provisioner](noobaa-obc-provisioner.md) - Method to claim a new/existing bucket.

Roadmap
- Operator Lifecycle Manager integration [](noobaa-olm.md)
- Scaling up/down S3 endpoints [](noobaa-endpoints.md)
- Multi-cluster [](noobaa-multicluster.md)
- Security [](noobaa-security.md)
