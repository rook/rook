# Using Rook
The `rook` client tool can be used to manage your Rook cluster once it is running as well as manage block, file and object storage.  See the sections below for details on how to configure each type of storage.  

If you don't yet have a Rook cluster running, refer to our [Quickstart Guides](../../README.md#quickstart-guides).

## Block Storage
1. Create a new volume image (10MB)

    ```bash
    $ rook block create --name test --size 10485760
    ```

2. Mount the block volume and format it

    ```bash
    sudo -E rook block mount --name test --path /tmp/rook-volume
    sudo chown $USER:$USER /tmp/rook-volume
    ```

3. Write and read a file

    ```bash
    echo "Hello Rook!" > /tmp/rook-volume/hello
    cat /tmp/rook-volume/hello
    ```

4. Cleanup

    ```bash
    sudo rook block unmount --path /tmp/rook-volume
    ```

## Shared File System
1. Create a shared file system

    ```bash
    rook filesystem create --name testFS
    ```

2. Verify the shared file system was created

   ```bash
   rook filesystem ls
   ```

3. Mount the shared file system from the cluster to your local machine

   ```bash
   rook filesystem mount --name testFS --path /tmp/rookFS
   sudo chown $USER:$USER /tmp/rookFS
   ```

4. Write and read a file to the shared file system

   ```bash
   echo "Hello Rook!" > /tmp/rookFS/hello
   cat /tmp/rookFS/hello
   ```

5. Unmount the shared file system (this does **not** delete the data from the cluster)

   ```bash
   rook filesystem unmount --path /tmp/rookFS
   ```

6. Cleanup the shared file system from the cluster (this **does** delete the data from the cluster)

   ```
   rook filesystem delete --name testFS
   ```

## Object Storage
1. Create an object storage instance in the cluster

   ```bash
   rook object create
   ```

2. Create an object storage user

   ```bash
   rook object user create rook-user "A rook rgw User"
   ```

3. Get the connection information for accessing object storage

   ```bash
   eval $(rook object connection rook-user --format env-var)
   ```

4. Use an S3 compatible client to create a bucket in the object store

   ```bash
   s3cmd mb --no-ssl --host=${AWS_ENDPOINT} --host-bucket=  s3://rookbucket
   ```

5. List all buckets in the object store

   ```bash
   s3cmd ls --no-ssl --host=${AWS_ENDPOINT} --host-bucket=
   ```

6. Upload a file to the newly created bucket

   ```bash
   echo "Hello Rook!" > /tmp/rookObj
   s3cmd put /tmp/rookObj --no-ssl --host=${AWS_ENDPOINT} --host-bucket=  s3://rookbucket
   ```

7. Download and verify the file from the bucket

   ```bash
   s3cmd get s3://rookbucket/rookObj /tmp/rookObj-download --no-ssl --host=${AWS_ENDPOINT} --host-bucket=
   cat /tmp/rookObj-download
   ```
