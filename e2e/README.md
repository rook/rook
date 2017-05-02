# Rook Test Framework
 
The Rook Test Framework is a Go project that that uses a Docker container with Kubernetes
tools and Docker pre-installed to quickly create a 3 node Kubernetes cluster using Docker in 
Docker (DinD) techniques, installs Rook and then runs automation tests to verify its functionality.
 
##Requirements
1. Docker version => 1.2 && < 17.0
2. Ubuntu Host 16 (currently the framework has only been tested on this version)
 
##Instructions 
On your Ubuntu box do the following;
1.   Be sure to allocate a minimum of 4 processors to your vm
2.   Navigate to the root directory of the repo and run the following:;
         ```e2e/scripts/smoke_test.sh```

At this point a Docker container with the base image of "quay.io/quantum/rook-test" is created, a
Kubernetes 3 node cluster is created using Kubeadm, that is visible from the Docker host. The 
nodes can be identified by the following container names (kube-master, kube-node1 and kube-node2).
All tests with the name 'SmokeSuite' in the method will execute and the results will be of a junit
type output to the e2e/results directory.


### Cleanup
The Rook Test Framework normally cleans up all the containers it creates to setup the environment
and to run the tests. If for some reason the cleanup of the Rook Test framework should fail, the easiest way to manually
cleanup the environment is by doing the following;

1. Delete the docker container that uses an image named "quay.io/quantum/rook-test"
2. Run the script rook/e2e/framework/manager/scripts/rook-dind-cluster-v1.6.sh clean