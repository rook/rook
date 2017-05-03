# Using Rook
The `rook` client tool can be used to manage your Rook cluster once it is running as well as manage block, file and object storage.  See the sections below for details on how to configure each type of storage.  

If you don't yet have a Rook cluster running, refer to our [Quickstart Guides](README.md).

## Block Storage
1. Create a new volume image (10MB)

    ```bash
    rook block create --name test --size 10485760
    ```

2. Map the block volume and format it and mount it

    ```bash
    sudo -E rook block map --name test --format --mount /tmp/rook-volume
    sudo chown $USER:$USER /tmp/rook-volume
    ```

3. Write and read a file

    ```bash
    echo "Hello Rook!" > /tmp/rook-volume/hello
    cat /tmp/rook-volume/hello
    ```

4. Cleanup

    ```bash
    sudo rook block unmap --mount /tmp/rook-volume
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

4. Get rook object connection details

   ```bash
   rook object connection rook-user --format env-var
   ```

5. Install editor of your choice , here we are installing vim
   ```bash
   apt install vim
   ```

6. Configure s3cmd
   ```bash
   s3cmd --configure
   ```
   ```
   Enter new values or accept defaults in brackets with Enter.
   Refer to user manual for detailed description of all options.

   Access key and Secret key are your identifiers for Amazon S3. Leave them empty for using the env variables.
   Access Key: < Add your Access Key Here >
   Secret Key: < Add your Secret Key Here >
   Default Region [US]:

   Encryption password is used to protect your files from reading
   by unauthorized persons while in transfer to S3
   Encryption password:
   Path to GPG program [/usr/bin/gpg]:

   When using secure HTTPS protocol all communication with Amazon S3
   servers is protected from 3rd party eavesdropping. This method is
   slower than plain HTTP, and can only be proxied with Python 2.7 or newer
   Use HTTPS protocol [Yes]: no

   On some networks all internet access must go through a HTTP proxy.
   Try setting it here if you can't connect to S3 directly
   HTTP Proxy server name:

   New settings:
     Access Key: W1AALV375BM0PE6BHLJQ
     Secret Key: GTRaEqsjMeXLLbGaICMOk636G7E6YhIcb2OBB7el
     Default Region: US
     Encryption password:
     Path to GPG program: /usr/bin/gpg
     Use HTTPS protocol: False
     HTTP Proxy server name:
     HTTP Proxy server port: 0

   Test access with supplied credentials? [Y/n] n

   Save settings? [y/N] y
   Configuration saved to '/root/.s3cfg'
   ```
7. Edit s3cmd configuration file and provide values for ```host``` and ```host-bucket```

8. Use an S3 compatible client to create a bucket in the object store

   ```bash
   s3cmd mb --no-ssl --host=${AWS_ENDPOINT} --host-bucket=  s3://rookbucket
   ```

9. List all buckets in the object store

   ```bash
   s3cmd ls --no-ssl --host=${AWS_ENDPOINT}
   ```

10. Upload a file to the newly created bucket

   ```bash
   echo "Hello Rook!" > /tmp/rookObj
   s3cmd put /tmp/rookObj --no-ssl --host=${AWS_ENDPOINT} --host-bucket=  s3://rookbucket
   ```

11. Download and verify the file from the bucket

   ```bash
   s3cmd get s3://rookbucket/rookObj /tmp/rookObj-download --no-ssl --host=${AWS_ENDPOINT} --host-bucket=
   cat /tmp/rookObj-download
   ```
