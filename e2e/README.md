# Rook Test Framework
 
The Rook Test Framework is a Go project that that uses a Docker container with Kubernetes
tools and Docker pre-installed to quickly create a three node Kubernetes cluster using Docker in 
Docker (DinD) techniques, installs Rook and then runs automation tests to verify its functionality.
 
## Requirements

1. Docker version => 1.2 && < 17.0
2. Ubuntu Host 16 (currently the framework has only been tested on this version)
 
## Instructions 

On your Ubuntu box do the following;
1.   Be sure to allocate a minimum of 4 processors to your vm
2.   Navigate to the root directory of the repo and run the following:
         ```e2e/scripts/smoke_test.sh```

At this point a Docker container with the base image of ```quay.io/quantum/rook-test``` is created, a
Kubernetes three node cluster is created using Kubeadm, that is visible from the Docker host. The 
nodes can be identified by the following container names (kube-master, kube-node1 and kube-node2).
All tests with the name `SmokeSuite` in the method will execute and the results will be of a junit
type output to the `e2e/results` directory.

## Run Tests
Rook Framework as several configuration options, that can be used to run install rook and run tests, or just run 
tests against rook that is already installed.
  
#### Test parameters
The following parameters are available while running tests

 Parameter | Description | Possible values | Default
 --- |--- | --- | --- 
rook_platform| platform rook needs to be installed on  | kubernetes | kubernetes
k8s_version  | version of Kubernetes to be installed  | v1.6  | v1.6
rook_version | rook version/tag to be installed | valid rook tag |master-latest 
skip_install_rook | skips installing rook (if already installed) | true or false  | false

if install_rook flag is set to false, then all the other flags are ignored,
and tests are run without rook being installed and set up. Use this flag to run tests against
a pre-installed/configured rook. 

#### Test set up
  Run ```glide install ``` on dir ```~e2e/```

#### Install Rook and run Test
To install rook and run tests use the command 
```
go test -run TestObjectStorageSmokeSuite github.com/rook/rook/e2e/tests/smoke
```
This uses the default  ```--skip_install_rook=false``` flag.  Then Based on other flags, rook is installed and tests
are run. e.g.
```
go test -run TestObjectStorageSmokeSuite github.com/rook/rook/e2e/tests/smoke --rook_version=v3.0.1
```

#### Run Tests on pre-installed/exisitng Rook
To run tests against a pre-installed rook set  ```--skip_install_rook=true``` . 
Setting skip_install_rook to true will ignore all other flags and run tests without installing rook first.

e.g.
```
go test -run TestFileStorageSmokeSuite github.com/rook/rook/e2e/tests/smoke --skip_install_rook=true
```


Prerequisites :
* Go installed and GO_PATH set up
* Glide installed 
* when running tests locally, make sure Kubectl is accessible globally, test framework uses kubectl to do setup work. 
* You can run tests using IDE of your choice, the the flags are configured in ```e2e/framework/objects/environment_manifest.go```
file. Update the file to set default values as you see fit and then run tests from IDE directly. 


## Cleanup

The Rook Test Framework normally cleans up all the containers it creates to setup the environment
and to run the tests. If for some reason the cleanup of the Rook Test framework should fail, the easiest way to manually
cleanup the environment is by doing the following

1. Delete the docker container that uses an image named `quay.io/rook/rook-test`
2. Run the script ```rook/e2e/framework/manager/scripts/rook-dind-cluster-v1.6.sh clean```
