
# Rook Flexvolume

## Flexvolume
Flexvolume enables users to write their own drivers and add support for their volumes in Kubernetes. Refer to [Kubernetes Flexvolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) for more information.

## Install Rook Flexvolume

To install the Rook Flexvolume, copy the Rook Flevolume [driver](/demo/kubernetes/flexvolume/rook) to `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/kubernetes.io~rook`. Make sure this file is executable.

Restart kubelet to load the driver.

## Running the Example

Once the Rook Flexvolume plugin is loaded by kubelet, you can start consuming Rook volumes in your applications. Follow this steps:

1. Deploy rook. You can follow this documentation to [deploy rook](kubernetes.md).
1. Create a pool. This can be done by creating a `Rookpool` resource (See documentation on [Rookpool TPR](pool-tpr.md)  or using the [Rook Client](client.md).
1. Create a volume using the Rook Client. The volume has to be manually created because Flexvolume does not support dynamic provisioning of volumes yet (currently being proposed [here](https://github.com/kubernetes/kubernetes/issues/32543)).
1. Create a secret that holds your Ceph key. The secret must be of type `kubernetes.io/rook`.
1. Edit the sample [mysql.yaml](/demo/kubernetes/flexvolume/mysql.yaml) and provide the `Rook API`, `block`, `pool` and `user`. If you provide a `fsType`, the volume will be formatted to that type.
1. Create the sample application. `kubectl create -f mysql.yaml`. You should see the volume being attached and mounted to the pod.

If you remove the pod, the volume will be detached and unmounted from the pod. But it will not be deleted.
