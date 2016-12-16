# Rook on Kubernetes

A demo of persistent block storage from rook within a [Kubernetes](http://kubernetes.io/) cluster. We will use a mysql service in Kubernetes as an example of using block storage from rook.

## Prerequisites

A running Kubernetes cluster is required. First clone <https://github.com/rook/coreos-kubernetes/tree/rook-demo> (rook-demo branch in the rook fork of the coreos-kubernetes repo):
```
git clone https://github.com/rook/coreos-kubernetes.git
cd coreos-kubernetes
git checkout rook-demo
```

The changes found in that branch that facilitate this demo scenario include:  
1. Devices created in the vagrant environment to provide storage in the cluster.  
2. Access to modprobe, necessary for using the rbd volume plugin (<https://github.com/kubernetes/kubernetes/issues/23924>).  

### Multi-node deployment

If you are using the rook-demo branch you can get a vagrant kubernetes cluster running like this (from the root of the repo):
```
cd multi-node/vagrant
vagrant up
export KUBECONFIG="$(pwd)/kubeconfig"
kubectl config use-context vagrant-multi
```

Then wait for the cluster to come up and verify that kubernetes is done initializing (be patient, it takes a bit):
```
kubectl get nodes
```

### Single-node deployment

This walkthrough is using the multi-node deployment. To instead use the single-node deployment, the main change to the instructions is the IP address.
Replace `172.17.4.101` with `172.17.4.99` in these instructions and in `rook.yml`.

## Starting rook
```
cd <rook>/demo/kubernetes
export TOKEN=$(curl -s -w "\n" 'https://discovery.etcd.io/new?size=1')
kubectl create configmap rookd --from-literal=discovery-token=$TOKEN
kubectl create -f rook.yml
```

This generates a discovery token so the nodes can find each other and then starts rook. `rookd` will start up on each node and form a cluster.

**Note**: In environments other than the coreos-kubernetes vagrant cluster, to have external access via the rook cli, the `rook.yml` will need to be edited and the `externalIPs` array will need to be changed to an appropriate externally routable ip to one of the cluster nodes.

## Using the rook storage with a rook client pod

First let's provision some useful storage in the rook cluster by using the `rook` command line client.  For convenience, there is a pod that contains the latest rook client.  It can be started in the cluster with:
```
kubectl create -f rook-client/rook-client.yml
```  

Starting the rook-client pod will take a bit of time to download the container, so you can check to see when it's ready with (it should be in the Running state):
```
kubectl get pod rook-client
```

Connect to the rook-client pod and verify the `rook` client can talk to the `rookd` cluster:
```
kubectl exec rook-client -it bash
rook node ls
```

At this point (optional), you can follow the steps in the rook [quickstart](/README.md#block-storage) to create and use block, file and object storage.

To use a block device in the kubernetes cluster you will first need to create a block image for that device.  **In your rook-client pod** from above, run:
```
rook block create --name demoblock --size 1073741824
```

Now that the block image has been created in the cluster, we can exit the `rook` client pod:
```
exit
```

## Running a pod with persistent rook storage
Now, **back on your host machine**, set a variable so subsequent commands can find the rook API server:
```
export ROOK_API_SERVER_ENDPOINT=172.17.4.201:8124
```
**NOTE:** If you are not using coreos-kubernetes vagrant, you need to substitute the IP from above that you put in the `rook.yml` for `ROOK_API_SERVER_ENDPOINT`

Now, we will want to fetch the rook secret that is used to mount rook block devices and store it in a Kubernetes secret. This only needs to be done once for a given cluster:
```
SECRET=$(curl -s $ROOK_API_SERVER_ENDPOINT/client | jq -r '.secretKey')
kubectl create secret generic rookd --from-literal=key=$SECRET --type kubernetes.io/rbd
```   

Then we must fetch the monitor endpoints and substitute them in to the `mysql.yml` file for the example mysql service and pipe it into `kubectl` to create the pod:
```
export MONS=$(curl -s $ROOK_API_SERVER_ENDPOINT/client | jq -c '.monAddresses')
sed 's#INSERT_HERE#'$MONS'#' mysql.yml | kubectl create -f -
```

Verify that the mysql pod has the rbd volume mounted (it may take a bit to first pull the mysql image):
```
kubectl describe pod mysql
```

With mysql pod running, we now have a pod using persistent storage from the `rook` cluster in the form of a block device. 

## Teardown demo
```
kubectl delete pod rook-client
kubectl delete pod mysql
kubectl delete secret rookd
kubectl delete configmap rookd
kubectl delete -f rook.yml
```

kubectl should show no pods running at all (be patient, they make take a bit to terminate completely).  You can walk through the demo again if you desire.

To fully clean up the environment (and **destroy all data**), run the following from the same directory you initially ran `vagrant up` in:
```
vagrant destroy -f
```

## Troubleshooting
Information about the host nodes that are running the kubernetes cluster (and pods deployed in it) can be found with:
```
kubectl get nodes
```
Before the kubernetes cluster is initialized, which does take some time, it is common to see an error message like:
> The connection to the server 172.17.4.101:443 was refused - did you specify the right host or port?

To get information about the pods that have been created/deployed to the kubernetes cluster, run:
```
kubectl get pods
NAME          READY     STATUS             RESTARTS   AGE
rookd-rglk7   1/1       Running            0          15m
rookd-s238a   0/1       CrashLoopBackOff   7          15m
```
Of particular note is the STATUS column.  Ideally, the pods will be in the 'Running' status.  'ContainerCreating' is OK as well since it means the container is downloading or initializing.  'Error' and 'CrashLoopBackOff' are not good, so more investigation into any particular pods with those status values will be needed, using the commands below. 


Details about the config, status and recent events for a pod can be found with the `describe pod` command, like so: 
```
kubectl describe pod <NAME>
```

Each pod generates log output that can be valuable for identifying issues.  To get logs scoped to a specific pod, the `logs` command can be used:
```
kubectl logs <NAME>
```

Combining some shell scripting, we can get the last 10 lines of the logs for *each* `rookd` pod with this command:
```
for p in `kubectl get pods | grep rookd | awk '{print $1}'`; do echo $p; kubectl logs $p | tail -10; done
```

Similarly, we can call `describe pod` on each `rookd` pod with this command:
```
for p in `kubectl get pods | grep rookd | awk '{print $1}'`; do echo $p; kubectl describe pod $p; done
```

If all else fails, in the vagrant kubernetes environment, we can `ssh` directly to a host in the cluster and use the host's tools to probe further.  For example, this can be run from the same directory where you ran `vagrant up`:
```
vagrant ssh w1
journalctl
```
Hosts where `rookd` will be deployed are `w1` and `c1`.

## TODO

* Kubernetes volume plugin for rook that avoids the need for the mon endpoints being hard-coded into the pod spec
* a better solution for the modprobe issue (<https://github.com/kubernetes/kubernetes/issues/23924>)
