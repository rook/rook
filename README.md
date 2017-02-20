![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)

## Open, Cloud Native, and Universal Distributed Storage

- [What is Rook?](#what-is-rook)
- [Status](#status)
- [Kubernetes](#kubernetes)
- [Rook Standalone Service](#rook-standalone-service)
- [Block, File and Object Storage](#block-file-and-object-storage)
- [Building](#building)
- [Design](#design)
- [Contributing](#contributing)
- [Contact](#contact)
- [Licensing](#licensing)

## What is Rook?

Rook is a distributed storage system designed for cloud native applications. It aims to be the storage solution for container-native app developers. 
Just like your container apps, Rook is designed to take advantage of the resource management and orchestration of the Kubernetes platform.
This allows Rook to share cluster resources with your apps, allowing it to dynamically scale as your apps scale and save the effort to manage storage infrastructure separately. 
Rook provides persistent storage for your Kubernetes apps - which is one of the largest barriers for developers to fully containerize their apps. 
Rook exposes file, block, and object storage on top of shared resource pools. Rook has minimal
dependencies and can be deployed in dedicated storage clusters or converged clusters. It's
self-managing, self-protecting, self-healing, and is designed to just work without teams of
engineers managing it. It scales from a single node, to multi-PB clusters spread geographically.
It's based on the [Ceph](http://ceph.com) project with over 10 years of production deployments in some of the
largest storage clusters in the world.

## Status

Rook is in **alpha** state. We're just getting started. Not all planned features are complete. The API
and other user-facing objects are subject to change. Backward-compability is not supported for this
release. See our [Roadmap](https://github.com/rook/rook/wiki/Roadmap) and [Issues](https://github.com/rook/rook/issues).
Please help us by [Contributing](CONTRIBUTING.md) to the project.

## Quickstart

Here's the quickest way to get going with Rook.

### Kubernetes

This example shows how to build a simple, multi-tier web application on Kubernetes using persistent volumes enabled by Rook.

#### Prerequisites

This example requires a running Kubernetes cluster. To make sure you have a Kubernetes cluster that is ready for `rook`, you can [follow these quick instructions](demo/kubernetes/README.md).

Note that we are striving for even more smooth integration with Kubernetes in the future such that `rook` will work out of the box with any Kubernetes cluster.

#### Deploy Rook

With your Kubernetes cluster running, Rook can be setup and deployed by simply deploying the [rook-operator](demo/kubernetes/rook-operator.yaml) deployment manifest.
You will find this manifest and all our example manifest files in the [demo/kubernetes](demo/kubernetes) folder.

```
cd demo/kubernetes
kubectl create namespace rook
kubectl create -f rook-operator.yaml
```

Use `kubectl` to list pods in the rook namespace. You should be able to see the following: 

```
$ kubectl -n rook get pod
NAME                            READY     STATUS    RESTARTS   AGE
mon0                            1/1       Running   0          1m
mon1                            1/1       Running   0          1m
mon2                            1/1       Running   0          1m
osd-n1sm3                       1/1       Running   0          1m
osd-pb0sh                       1/1       Running   0          1m
osd-rth3q                       1/1       Running   0          1m
rgw-1785797224-9xb4r            1/1       Running   0          1m
rgw-1785797224-vbg8d            1/1       Running   0          1m
rook-api-4184191414-l0wmw       1/1       Running   0          1m
rook-operator-349747813-c3dmm   1/1       Running   0          1m
```
**NOTE:** RGW (object storage gateway) is currently deployed by default but in the future will be done only when needed (see [#413](https://github.com/rook/rook/issues/413))

#### Provision Storage
Before Rook can start provisioning storage, a StorageClass needs to be created. This is used to specify information needed for Kubernetes to interoperate with Rook for provisioning persistent volumes.  Rook already creates a default admin and demo user, whose secrets are already specified in the sample [rook-storageclass.yaml](demo/kubernetes/rook-storageclass.yaml).

Now we just need to specify the Ceph monitor endpoints (requires `jq`):

```
export MONS=$(kubectl -n rook get pod mon0 mon1 mon2 -o json|jq ".items[].status.podIP"|tr -d "\""|sed -e 's/$/:6790/'|paste -s -d, -)
sed 's#INSERT_HERE#'$MONS'#' rook-storageclass.yaml | kubectl create -f -
``` 
**NOTE:** In the v0.4 release we plan to expose monitors via DNS/service names instead of IP address (see [#355](https://github.com/rook/rook/issues/355)), which will streamline the experience and remove the need for this step.

#### Consume the storage

Now that rook is running and integrated with Kubernetes, we can create a sample app to consume the block storage provisioned by rook. We will create the classic wordpress and mysql apps.
Both these apps will make use of block volumes provisioned by rook.

Start mysql and wordpress:

```
kubectl create -f mysql.yaml
kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Once the wordpress and mysql pods are in the `Running` state, get the cluster IP of the wordpress app and enter it in your brower:

```
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.  

**NOTE:** When running in a vagrant environment, there will be no external IP address to reach wordpress with.  You will only be able to reach wordpress via the `CLUSTER-IP` from inside the Kubernetes cluster.

#### Rook Client
You also have the option to use the `rook` client tool directly by running it in a pod that can be started in the cluster with:
```
kubectl create -f rook-client/rook-client.yml
```  

Starting the rook-client pod will take a bit of time to download the container, so you can check to see when it's ready with (it should be in the `Running` state):
```
kubectl -n rook get pod rook-client
```

Connect to the rook-client pod and verify the `rook` client can talk to the cluster:
```
kubectl -n rook exec rook-client -it bash
rook node ls
```

At this point (optional), you can follow the steps in the [Block, File and Object Storage section](#block-file-and-object-storage) to create and use those types of storage.

## Rook Standalone Service

Rook can also be deployed as a standalone service on any modern Linux host by running the following:

### Linux
1. Download the latest  binaries

    ```bash
    $ wget https://github.com/rook/rook/releases/download/v0.2.2/rook-v0.2.2-linux-amd64.tar.gz
    $ tar xvf rook-v0.2.2-linux-amd64.tar.gz
    ```

2. Start a one node Rook cluster

    ```bash
    $ ./rookd --data-dir /tmp/rook-test
    ```

### Vagrant

Rook is also easy to run with `vagrant` on CoreOS via `rkt`.

```
cd demo/vagrant
vagrant up
```

## Block, File and Object Storage

### Block Storage
1. Create a new volume image (10MB)

    ```bash
    $ rook block create --name test --size 10485760
    ```

2. Mount the block volume and format it

    ```bash
    sudo rook block mount --name test --path /tmp/rook-volume
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

### Object Storage
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

## Building

See [Building](https://github.com/rook/rook/wiki/Building) in the wiki for more details.

## Design

A rook cluster is made up of one or more nodes each running the Rook daemon `rookd`. Containers and Pods can
mount block devices and filesystems exposed by the cluster, or can use S3/Swift API for object storage. There is
also a REST API exposed by `rookd` as well as a command line tool called `rook`.

![Overview](Documentation/media/cluster.png)

The Rook daemon `rookd` is a single binary that is self-contained and has all that is needed to bootstrap, scale
and manage a storage cluster. `rookd` is typically compiled into a single static binary (just like most golang
binaries) or a dynamic binary that takes a dependency on mostly libc. It can run in minimal containers, alongside a
hypervisor, or directly on the host on most Linux distributions.

`rookd` uses an embedded version of Ceph for storing all data -- there are no changes to the data path. An embedded version
of Ceph was created specifically for Rook scenarios and has been pushed upstream. Rook does not attempt to maintain full fidelity
with Ceph, for example, most of the Ceph concepts like OSDs, MONs, placement groups, etc. are hidden. Instead Rook creates
a much simplified UX for admins that is in terms of physical resources, pools, volumes, filesystems, and buckets.

`rookd` embeds Etcd to store configuration and coordinate cluster-wide management operations. `rookd` will automatically
bootstrap Etcd, manage it, and scale it as the cluster grows. It's also possible to use an external Etcd instead of the embedded one
if needed.

Rook and etcd are implemented in golang. Ceph is implemented in C++ where the data path is highly optimized. We believe
this combination offers the best of both worlds.

See [Design](https://github.com/rook/rook/wiki/Design) wiki for more details.

## Contributing

We welcome contributions. See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, help us out by opening an [issue](https://github.com/rook/rook/issues).

## Contact

Please use the following to reach members of the community:

- Email: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Gitter: [rook/rook](https://gitter.im/rook/rook) for general project discussions or [rook-dev](https://gitter.im/rook/rook-dev) for development discussions.
- Twitter: [@rook_io](https://twitter.com/rook_io)

## Licensing

Rook and Etcd are under the Apache 2.0 license. Ceph is mostly under the LGPL 2.0 license. Some portions
of the code are under different licenses. The appropriate license information can be found in the headers
of the source files.
