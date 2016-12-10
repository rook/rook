# Rook on Kubernetes

A demo of persistent block storage from rook within a [Kubernetes](http://kubernetes.io/) cluster. We will use a mysql service in Kubernetes as an example of using block storage from rook.

## Prerequisites

A running Kubernetes cluster is required. See the <https://github.com/rook/coreos-kubernetes/tree/rook-demo> (rook-demo branch) in the rook fork of the coreos-kubernetes repo.
The changes to coreos-kubernetes necessary to support the scenario include:
1. Devices created in the vagrant environment to provide storage in the cluster.
2. Access to modprobe, necessary for using the rbd volume plugin (<https://github.com/kubernetes/kubernetes/issues/23924>).

### Multi-node deployment

If you are using the rook-demo branch you can get a vagrant kubernetes cluster running like this (from the root of the repo):

    cd multi-node/vagrant
    vagrant up
    export KUBECONFIG="$(pwd)/kubeconfig"
    kubectl config use-context vagrant-multi

Verify that kubernetes is done initializing.

    kubectl get nodes


### Single-node deployment

This walkthrough is using the multi-node deployment. To use the single-node deployment, the main change to the instructions is the ip address.
Replace `172.17.4.101` with `172.17.4.99` in these instructions and in `rook.yml`.

## Starting rook

    cd <rook>/demo/kubernetes
    export TOKEN=$(curl -s -w "\n" 'https://discovery.etcd.io/new?size=1')
    kubectl create configmap rookd --from-literal=discovery-token=$TOKEN
    kubectl create -f rook.yml

This generates a discovery token so the nodes can find each other and then starts rook. `rookd` will start up on each node and form a cluster.

**Note**: In environments other than the coreos-kubernetes vagrant cluster, to have external access via the rook cli, the `rook.yml` will need to be edited and the `externalIPs` array will need to be changed to an appropriate externally routable ip to one of the cluster nodes.

## Using the storage

First let's setup access to the rook cluster via the command line client.  You can download a pre-built from [github releases](https://github.com/rook/rook/releases) or [build from source](https://github.com/rook/rook/blob/master/build/README.md). (If you are not using coreos-kubernetes vagrant substitute the ip from above that you put in the `rook.yml` for `ROOK_API_SERVER_ENDPOINT`)

    export ROOK_API_SERVER_ENDPOINT=172.17.4.201:8124
    rook status
    
It may take a moment for the rook cluster to come up and `rook status` to complete successfully.  Once it is successful we will want to fetch the rook secret that is used to mount rook block devices and store it in a Kubernetes secret. This only needs to be done once for a given cluster.

    SECRET=$(curl -s $ROOK_API_SERVER_ENDPOINT/client | jq -r '.secretKey')
    kubectl create secret generic rookd --from-literal=key=$SECRET --type kubernetes.io/rbd

To use a block device in the kubernetes cluster you will first need to create a block image for that device.

    rook block create --name demoblock --size 1073741824

Then we must fetch the mon endpoints and substitute them in to the `mysql.yml` for the example mysql service and pipe it into `kubectl` to create the pod:
  
    export MONS=$(curl -s $ROOK_API_SERVER_ENDPOINT/client | jq -c '.monAddresses')
    sed 's#INSERT_HERE#'$MONS'#' mysql.yml | kubectl create -f -

Verify that the mysql pod has the rbd volume mounted.

    kubectl describe pod mysql

## Teardown demo

    kubectl delete pod mysql
    kubectl delete secret rookd
    kubectl delete configmap rookd
    kubectl delete -f rook.yml

## Todo

* Kubernetes volume plugin for rook that avoids the need for the mon endpoints being hard-coded into the pod spec
* a better solution for the modprobe issue (<https://github.com/kubernetes/kubernetes/issues/23924>)
