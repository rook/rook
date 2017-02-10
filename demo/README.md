# Standalone Rook

Rook can also run as a standalone service on any modern Linux host.

## Linux

On a modern Linux host run the following:

1. Download the latest  binaries

    ```bash
    $ wget https://github.com/rook/rook/releases/download/v0.2.2/rook-v0.2.2-linux-amd64.tar.gz
    $ tar xvf rook-v0.2.2-linux-amd64.tar.gz
    ```

2. Start a one node Rook cluster

    ```bash
    $ ./rookd --data-dir /tmp/rook-test
    ```

### Block Storage
1. In a different shell (in the same path) create a new volume image (10MB)

    ```bash
    $ ./rook block create --name test --size 10485760
    ```

2. Mount the block volume and format it

    ```bash
    sudo ./rook block mount --name test --path /tmp/rook-volume
    sudo chown $USER:$USER /tmp/rook-volume
    ```

3. Write and read a file

    ```bash
    echo "Hello Rook!" > /tmp/rook-volume/hello
    cat /tmp/rook-volume/hello
    ```

4. Cleanup

    ```bash
    sudo ./rook block unmount --path /tmp/rook-volume
    ```

### Shared File System
1. Create a shared file system

    ```bash
    ./rook filesystem create --name testFS
    ```

2. Verify the shared file system was created

   ```bash
   ./rook filesystem ls
   ```

3. Mount the shared file system from the cluster to your local machine

   ```bash
   ./rook filesystem mount --name testFS --path /tmp/rookFS
   sudo chown $USER:$USER /tmp/rookFS
   ```

4. Write and read a file to the shared file system

   ```bash
   echo "Hello Rook!" > /tmp/rookFS/hello
   cat /tmp/rookFS/hello
   ```

5. Unmount the shared file system (this does **not** delete the data from the cluster)

   ```bash
   ./rook filesystem unmount --path /tmp/rookFS
   ```

6. Cleanup the shared file system from the cluster (this **does** delete the data from the cluster)

   ```
   ./rook filesystem delete --name testFS
   ```

### Object Storage
1. Create an object storage instance in the cluster

   ```bash
   ./rook object create
   ```

2. Create an object storage user

   ```bash
   ./rook object user create rook-user "A rook rgw User"
   ```

3. Get the connection information for accessing object storage

   ```bash
   eval $(./rook object connection rook-user --format env-var)
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

## CoreOS

Rook is also easy to run on CoreOS either directly on the host or via rkt.

```
cd demo/vagrant
vagrant up
```
