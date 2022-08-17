# v1.10 Pending Release Notes

## Breaking Changes

- Remove support for Ceph Octopus (v15)

## Features

- The toolbox pod now uses the Ceph image directly instead of the Rook image
- Add support for AWS Server Side Encryption with (AWS-SSE:S3)[https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingServerSideEncryption.html] for RGW.
- Add option to specify custom endpoint list for cephobjectzone
