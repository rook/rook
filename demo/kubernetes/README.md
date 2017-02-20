
## Existing Kubernetes Cluster
If you already have a running Kubernetes cluster, you will also need to modify the kubelet service to bind mount `/sbin/modprobe` to allow access to `modprobe`. Access to modprobe is necessary for using the rbd volume plugin (<https://github.com/kubernetes/kubernetes/issues/23924>).
If using RKT, you can allow modprobe by following this [doc](https://github.com/coreos/coreos-kubernetes/blob/master/Documentation/kubelet-wrapper.md#allow-pods-to-use-rbd-volumes).

Instructions also copied here for convenience:  
Pods using the [rbd volume plugin](https://github.com/kubernetes/kubernetes/tree/master/examples/volumes/rbd) to consume data from ceph must ensure that the kubelet has access to modprobe. Add the following options to the `RKT_OPTS` env before launching the kubelet via kubelet-wrapper:

```ini
[Service]
Environment=KUBELET_VERSION=v1.5.3_coreos.0
Environment="RKT_OPTS=--volume modprobe,kind=host,source=/usr/sbin/modprobe \
  --mount volume=modprobe,target=/usr/sbin/modprobe \
  --volume lib-modules,kind=host,source=/lib/modules \
  --mount volume=lib-modules,target=/lib/modules \
  --uuid-file-save=/var/run/kubelet-pod.uuid"
...
```

Note that the kubelet also requires access to the userspace `rbd` tool that is included only in hyperkube images tagged `v1.3.6_coreos.0` or later.

## New Local Kubernetes Cluster
Alternatively, for a quick start with a new local cluster, use the Rook fork of [coreos-kubernetes](https://github.com/rook/coreos-kubernetes). This will bring up a multi-node Kubernetes cluster with `vagrant` and CoreOS virtual machines ready to use `rook` immediately.
```
git clone https://github.com/rook/coreos-kubernetes.git
cd coreos-kubernetes/multi-node/vagrant
vagrant up
export KUBECONFIG="$(pwd)/kubeconfig"
kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):

```
kubectl cluster-info
```

If you see a url response, your cluster is ready to go.

## Using Rook in Kubernetes
Now that you have a Kubernetes cluster running, you can start using `rook` with [these steps](../../README.md#kubernetes)