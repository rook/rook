---
title: Rook Client
weight: 74
indent: true
---

# Rook Client

** **rookctl has been deprecated. [CRDs](crds.md) are the recommended way to configure the cluster.** **

The `rookctl` client tool can be used to manage your Rook cluster once it is running as well as manage block, file and object storage.  See the sections below for details on how to configure each type of storage.

If you don't yet have a Rook cluster running, refer to our [Quickstart Guides](../README.md#quickstart-guides).

`rookctl` can be accessed in the following ways:

- Kubernetes: Start the [toolbox](toolbox.md) pod

## Block Storage

1. Create a new pool and volume image (10MB)

    ```bash
    rookctl pool create --name rbd --replica-count 1
    rookctl block create --name test --size 10485760
    ```

1. Map the block volume and format it and mount it

    ```bash
    # If running in the toolbox container, no need to run privileged
    rookctl block map --name test --format --mount /tmp/rook-volume
    ```

1. Write and read a file

    ```bash
    echo "Hello Rook!" > /tmp/rook-volume/hello
    cat /tmp/rook-volume/hello
    ```

1. Cleanup

    ```bash
    # If running in the toolbox container, no need to run privileged
    rookctl block unmap --mount /tmp/rook-volume
    ```

## Shared File System

1. Create a shared file system

    ```bash
    rookctl filesystem create --name testfs --data-replica-count 1 --metadata-replica-count 1
    ```

1. Verify the shared file system was created

   ```bash
   rookctl filesystem ls
   ```

1. Mount the shared file system from the cluster to your local machine

   ```bash
   # If running in the toolbox container, no need to run privileged
   rookctl filesystem mount --name testfs --path /tmp/rookFS
   ```

1. Write and read a file to the shared file system

   ```bash
   echo "Hello Rook!" > /tmp/rookFS/hello
   cat /tmp/rookFS/hello
   ```

1. Unmount the shared file system (this does **not** delete the data from the cluster)

   ```bash
   # If running in the toolbox container, no need to run privileged
   rookctl filesystem unmount --path /tmp/rookFS
   ```

1. Cleanup the shared file system from the cluster (this **does** delete the data from the cluster)

   ```
   rookctl filesystem delete --name testfs
   ```

## Object Storage

1. Create an object storage instance in the cluster

   ```bash
   rookctl object create -n my-store --data-replica-count 1 --metadata-replica-count 1
   ```

1. Create an object storage user

   ```bash
   rookctl object user create my-store rook-user "my object store user"
   ```

### Consume the Object Storage

Use an S3 compatible client to create a bucket in the object store. If you are running in Kubernetes,
the s3cmd tool is included in the [Rook toolbox](toolbox.md) pod.

1. Get the connection information for accessing object storage

   ```bash
   eval $(rookctl object connection my-store rook-user --format env-var)
   ```

1. Create a bucket in the object store

   ```bash
   s3cmd mb --no-ssl --host=${AWS_HOST} --host-bucket=  s3://rookbucket
   ```

1. List buckets in the object store

   ```bash
   rookctl object bucket list my-store
   ```

1. Upload a file to the newly created bucket

   ```bash
   echo "Hello Rook!" > /tmp/rookObj
   s3cmd put /tmp/rookObj --no-ssl --host=${AWS_HOST} --host-bucket=  s3://rookbucket
   ```

1. Download and verify the file from the bucket

   ```bash
   s3cmd get s3://rookbucket/rookObj /tmp/rookObj-download --no-ssl --host=${AWS_HOST} --host-bucket=
   cat /tmp/rookObj-download
   ```
