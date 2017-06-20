# Shared File System Quickstart

A shared file system can be mounted read-write from multiple pods. This may be useful for applications which can be clustered using a shared filesystem. 

This example runs a shared file system for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

### Prerequisites

This guide assumes you have created a Rook cluster and pool as explained in the main [Kubernetes guide](kubernetes.md)

## Rook Client
Setting up the Rook file system requires running `rookctl` commands with the [Rook client](kubernetes.md#rook-client). This will be simplified in the future with a TPR for the file system.

## Create the File System
Create the file system with the default pools.
```bash
rookctl filesystem create --name registryFS
```

If you are consuming the filesystem from a namespace other than `rook` you will need to copy the key to the desired namespace. 
In this example we are copying to the `kube-system` namespace.

```bash
kubectl get secret rook-admin -n rook -o json | jq '.metadata.namespace = "kube-system"' | kubectl apply -f -
```

### Optional: Adjust pool parameters

By default the pools do not have any redundancy. To create another copy of the data, let's set the replication to 2. 

First you will need to launch the [Rook toolbox](toolbox.md#running-the-toolbox-in-kubernetes) in order to run `ceph` commands.

Now from the toolbox pod we can modify the pool size
```bash
ceph osd pool set registryFS-data size 2
ceph osd pool set registryFS-metadata size 2
```

## Deploy the Application

The kube-registry yaml is defined [here](/demo/kubernetes/kube-registry.yaml). We will need to update the yaml with the monitor IP addresses with the following commands.
In the future this step will be improved with a Rook volume plugin.
```bash
cd demo/kubernetes
export MONS=$(kubectl -n rook get pod mon0 mon1 mon2 -o json|jq ".items[].status.podIP"|tr -d "\""|sed -e 's/$/:6790/'|paste -s -d, -)
sed "s/INSERT_MONS_HERE/$MONS/g" kube-registry.yaml | kubectl create -f -
```

You now have a docker registry which is HA with persistent storage.

### Test the storage

Once you have pushed an image to the registry (see the [instructions](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry) to expose and use the kube-registry), verify that kube-registry is using the filesystem that was configured above by mounting the shared file system in the rook-client pod. 

```bash
# Start and connect to the rook-client pod
kubectl create -f rook-client.yaml
kubectl -n rook exec rook-client -it sh

# Mount the same filesystem that the kube-registry is using
mkdir /tmp/registry
rookctl filesystem mount --name registryFS --path /tmp/registry

# If you have pushed images to the registry you will see a directory called docker
ls /tmp/registry 

# Cleanup the filesystem mount
rookctl filesystem unmount --path /tmp/registry
rmdir /tmp/registry
```

### Teardown
To clean up all the artifacts created by the file system demo:
```bash
kubectl -n kube-system delete secret rook-admin
kubectl delete -f kube-registry.yaml
```
