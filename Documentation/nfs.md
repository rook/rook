---
title: Network File System (NFS)
weight: 28
indent: true
---

# Network File System (NFS)

NFS allows remote hosts to mount file systems over a network and interact with those file systems as though they are mounted locally. This enables system administrators to consolidate resources onto centralized servers on the network.

## Prerequisites

1. A Kubernetes cluster is necessary to run the Rook NFS operator.
To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).
2. The volume that needs to be exported needs to be attached to NFS server pod via PVC. The volumes that can be attached are Host Path, AWS Elastic Block store, GCE Persistent Disk, CephFS, RBD etc.
The limitations of these volumes also apply while they are shared by NFS. The limitation and other details about these volumes can be [found here](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

You can [follow these instructions](quickstart.md) to deploy a sample Rook Ceph cluster that can be attached to the NFS server pod for sharing.
After the Rook Ceph cluster is up and running, create a PVC to consume it using the following command:
```console
cd cluster/examples/kubernetes/nfs
kubectl create -f ceph-pvc.yaml
```

## Deploy NFS Operator

First deploy the Rook NFS operator using the following commands:

```console
cd cluster/examples/kubernetes/nfs
kubectl create -f operator.yaml
```

You can check if the operator is up and running with:

```console
 kubectl -n rook-nfs-system get pod
```

## Create and Initialize NFS Server

Now that the operator is running, we can create an instance of a NFS server by creating an instance of the `nfsexports.nfs.rook.io` resource.
The resource values are used to configure the NFS server and export.
Full details of the configuration option can be found in the [NFS CRD documentation](nfs-crd.md).

When you are ready to create a nfs export, simply run:

```console
kubectl create -f nfs.yaml
```

We can verify that a Kubernetes object has been created that represents our new NFS export with the command below.

```console
kubectl -n rook-nfs get nfsexports.nfs.rook.io
```

To check if the NFS server is running:

```console
kubectl -n rook-nfs get pod -l app=rook-nfs
```

## Accessing the Export

To access the export, first create a storageclass:

```console
kubectl create -f storageclass.yaml
```
Then a PVC is used to access this storage.

## Consume the Export

There is an example of a web server app that utilizes the NFS Export to store data of its document root.
```console
cd ../
kubectl create -f httpd.yaml
```
To check if the PVC is attached run the following command:
```console
kubectl get pvc
```

## Teardown

To clean up all resources associated with this walk-through, you can run the commands below.

```console
kubectl delete -f nfs.yaml
kubectl delete -f operator.yaml
kubectl delete -f ceph-pvc.yaml
```

## Troubleshooting

If the cluster does not come up, the first step would be to examine the operator's logs:

```console
kubectl -n rook-nfs-system logs -l app=rook-nfs-operator
```