# Shared File System Quickstart

A shared file system can be mounted read-write from multiple pods. This may be useful for applications which can be clustered using a shared filesystem. 

This example runs a shared file system for the [kube-registry](https://github.com/kubernetes/kubernetes/tree/master/cluster/addons/registry).

### Prerequisites

This guide assumes you have created a Rook cluster and pool as explained in the main [Kubernetes guide](kubernetes.md)

## Rook Client
Setting up the Rook file system currently requires the Rook client. This will be simplified in the future with a TPR for the object stores.

```bash
kubectl create -f rook-client/rook-client.yml

# Starting the pod may take a couple minutes, so check to see when it's ready:
kubectl -n rook get pod rook-client

# Connect to the rook-client pod 
kubectl -n rook exec rook-client -it bash

# Confirm the rook client can connect to the cluster
rook status
```

## Create the File System
Create the file system with the default pools.
```bash
rook filesystem create --name registryFS
```

### Optional: Adjust pool paramaters

By default the pools do not have any redundancy. To create another copy of the data, let's set the replication to 2. 

First we will launch the [Rook toolbox](toolbox.md#running-the-toolbox) in order to run `ceph` commands.
```bash
# Start the Rook toolbox in order to run Ceph commands (the yml is found in the toolbox folder)
cd toolbox
kubectl create -f rook-tools.yml

# Verify the toolbox is running
kubectl -n rook get pod rook-tools

# Connect to the toolbox
kubectl -n rook exec -it rook-tools bash
```

Now we can modify the pool size
```bash
ceph osd pool set registryFS-data size 2
ceph osd pool set registryFS-metadata size 2
```

### Optional: Copy admin key to desired namespace

If you are consuming the filesystem from a namespace other than `rook` you will need to copy the key to the desired namespace. 
In this example we are copying to the `kube-system` namespace.

```bash
kubectl get secret rook-admin -n rook -o json | jq '.metadata.namespace = "kube-system"' | kubectl apply -f -
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

Verify that kube-registry is using the filesystem that was configured above.

```bash
# Start the rook toolbox
kubectl -n rook exec rook-tools -it bash

# Mount the same filesystem that the kube-registry is using
mkdir /tmp/registry
rook filesystem mount --name registryFS --path /tmp/registry

# Here you should see a directory called docker created by the registry
ls /tmp/registry 

# Cleanup the filesystem mount
rook filesystem unmount --path /tmp/registry
rmdir /tmp/registry
```

### Teardown
To clean up all the artifacts created by the file system demo:
```bash
kubectl -n kube-system delete secret rook-admin
kubectl delete -f kube-registry.yaml
```
