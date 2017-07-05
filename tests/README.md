# Rook Test Framework

The Rook Test Framework is used to run end to end and integration tests on rook. The framework depends on kubernetes
that is running. The framework also provides scripts for starting kubernetes using kubeadm or minikube, so users can 
quickly spin a kubernetes platform. The Kubeadm-dind.sh script starts a 3 node kubernetes(using docker in docker technique) 
and the minikube.sh starts a single node kubernetes. The Test framework is designed to install rook, run tests and uninstall rook

## Requirements

1. Docker version => 1.2 && < 17.0
2. Ubuntu Host 16 (currently the framework has only been tested on this version)
3. Kubernetes with kubectl configured
4. Rook

## Instructions

###Set up
#### Install Kubernetes
Users can choose any kubernetes flavor of their choice.  The Test framework only depends on kubectl being configured. 
The framework also provides scripts to install Kubernetes. Run [kubeadm-dind.sh](/tests/scripts/kubeadm-dind.sh) to setup
a three node kubernetes cluster using kubeadm(using docker in docker. each node is a docker container running kubernetes) 
or run [minikube.sh](/tests/scripts/minikube.sh)to set up a single node kubernetes using kubeadm. 
  
  Both the start up scrits are designed to start kubrnetes, make sure rbd is working and copy latest rook and toolbox images
  to the kubernetes containers.


### Run Tests
From the root do the following
1. Build rook : make build
2. Start kubernetes : using [kubeadm-dind.sh](/tests/scripts/kubeadm-dind.sh) or [minikube.sh](/tests/scripts/minikube.sh)
    run ```kube-didn.sh up``` or ```minikube.sh up```
    
3. Run integration tests : ```make test-integration``` 


### Test parameters
The following parameters are available while running tests

 Parameter | Description | Possible values | Default
 --- |--- | --- | ---
rook_platform| platform rook needs to be installed on  | kubernetes | kubernetes
k8s_version  | version of Kubernetes to be installed  | v1.5,v1.6 and v1.7  | v1.6
rook_image | rook image name to be installed | valid image name | rook/rook
toolbox_image | toolbox image name to be installed | valid image name | rook/toolbox
skip_install_rook | skips installing rook (if already installed) | true or false  | false
load_parallel_runs | performs concurrent operations (optional used for load test) | any number | 20

if install_rook flag is set to false, then all the other flags are ignored,
and tests are run without rook being installed and set up. Use this flag to run tests against
a pre-installed/configured rook.

### Running Tests with paramaters.

#### using make 
##### To run all integration Tests run 
```
make test-integration

```

##### To run all integration Tests on a specific suite. 
```
make test-integration SUITE=SmokeSuite

```
runs all tests that match regex


#### using go Test
##### To run all tests in a package :
```
go test  github.com/rook/rook/e2e/tests/smoke
```
runs all tests under /tests/smoke folder. 

##### To run specific tests : 
```
go test -run SmokeSuite github.com/rook/rook/tests/smoke
```
which runs all tests that match regex SmokeSuite in /tests/smoke folder 


##### To run specific without installing rook : 
```
go test -run SmokeSuite github.com/rook/rook/tests/smoke --skip_install_rook=true
```
if skip_install_rook flag is set to true, then rook is not uninstalled either. 

#### Run Longhaul Tests
Using go test `-count` option, any tests can be repeated n number of times to simulate load or longhaul test. Although 
any test can be converted to a longhaul test, it's ideal to run integration tests under load for extended period to simulate 
real world scenarios. Also a new custom test flag `load_parallel_runs` is added to control the number of concurrent operations being performed.
For example look at the [block long haul test](/tests/block/k8s/longhaul/basicBlockonghaul_test.go)
 
 To run a longhaul test you can run any integration test with `-count` and `--load_parallel_runs` options
 e.g.
 ```
 go test -run TestK8sBlockLongHaul github.com/rook/rook/e2e/tests/block/k8s/longhaul --load_parallel_runs=20 -count=1000
 ```

Prerequisites :
* Go installed and GO_PATH set up
* Glide installed 
* when running tests locally, make sure Kubectl is accessible globally, test framework uses kubectl to do setup work. 
* You can run tests using IDE of your choice, the the flags are configured in ```e2e/framework/objects/environment_manifest.go```
file. Update the file to set default values as you see fit and then run tests from IDE directly. 


### Cleanup
Test test framework is designed to uninstall rook after every test. If you using the --skip_install_rook flag to you might want to 
clean up rook manually.

To stop kubernetes  run ```kubeadm-dind.sh clean``` or ```minikube.sh clean```
