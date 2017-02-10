![logo](Documentation/media/logo.png?raw=true "Rook")

[![Build Status](https://jenkins.rook.io/buildStatus/icon?job=rook/rook/master)](https://jenkins.rook.io/blue/organizations/jenkins/rook%2Frook/activity)

## Open, Cloud Native, and Universal Distributed Storage

- [What is Rook?](#what-is-rook)
- [Status](#status)
- [Kubernetes](#kubernetes)
- [Rook Standalone Service](#rook-standalone-service)
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

This example requires a running Kubernetes cluster. You will also need to modify the kubelet service to bind mount `/sbin/modprobe` to allow access to `modprobe`. Access to modprobe is necessary for using the rbd volume plugin (<https://github.com/kubernetes/kubernetes/issues/23924>).
If using RKT, you can allow modprobe by following this [doc](https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md#allow-pods-to-use-rbd-volumes).  

For a quick start, use Rook fork for [coreos-kubernetes](https://github.com/rook/coreos-kubernetes). This will bring up a multi-node Kubernetes cluster, configured for using the rbd volume plugin.

```
$ git clone https://github.com/rook/coreos-kubernetes.git
$ cd coreos-kubernetes/multi-node/vagrant
$ vagrant up
$ export KUBECONFIG="$(pwd)/kubeconfig"
$ kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):

```
$ kubectl cluster-info
```

If you see a url response, you are ready to go.

#### Deploy Rook

Rook can be setup and deployed in Kubernetes by simply deploying the [rook-operator](https://github.com/rook/rook/blob/master/demo/kubernetes/rook-operator.yaml) deployment manifest.
You will this manifest and all our example manifests files in the [demo/kubernetes](https://github.com/rook/rook/blob/master/demo/kubernetes) folder.

```
$ cd demo/kubernetes
$ kubectl create namespace rook
$ kubectl create -f rook-operator.yaml
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

#### Provision Storage
Before Rook can start provisioning storage, a StorageClass needs to be created. This is used to specify the storage privisioner, parameters, admin secret and other information needed for Kubernetes to interoperate with Rook for provisioning persistent volumes.
Rook already creates a default admin and demo user, whose secrets are already specified in the sample [rook-storageclass.yaml](https://github.com/rook/rook/blob/master/demo/kubernetes/rook-storageclass.yaml).

However, before we proceed, we need to specify the Ceph monitor endpoints. You can find them by running this line (you will need `jq`). This will generated a comma-separated list of monitor IPs and ports `6790`. Add this list into the `monitors` param of the `rook-storageclass.yaml`

```
$ kubectl -n rook get pod mon0 mon1 mon2 -o json|jq .items[].status.podIP|tr -d "\""|sed -e 's/$/:6790/'|paste -s -d, -
10.2.2.80:6790,10.2.1.83:6790,10.2.0.47:6790
``` 

Create Rook Storage class:

```
$ kubectl create -f rook-storageclass.yaml
```

#### Consume the storage

Now that rook is running and integrated with Kubernetes, we can create a sample app to consume the block storaged provisioned by rook. We will create the classic wordpress and mysql apps.
Both these apps will make use of block volumes provisioned by rook.

Start mysql and wordpress:

```
$ kubectl create -f mysql.yaml
$ kubectl create -f wordpress.yaml
```

Both of these apps create a block volume and mount it to their respective pod. You can see the Kubernetes volume claims by running the following:

```
$ kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
mysql-pv-claim   Bound     pvc-95402dbc-efc0-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
wp-pv-claim      Bound     pvc-39e43169-efc1-11e6-bc9a-0cc47a3459ee   20Gi       RWO           1m
```

Get the cluster IP of the wordpress app and enter it in your brower:

```
$ kubectl get svc wordpress
NAME        CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
wordpress   10.3.0.155   <pending>     80:30841/TCP   2m
```

You should see the wordpress app running.

## Rook Standalone Service

Rook can also be deployed as a standalone service on any modern Linux host. Refer [here](https://github.com/kokhang/rook/tree/update-readme/demo/README.md) for steps on how to run Rook on a Linux host.

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
