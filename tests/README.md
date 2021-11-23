# Rook Test Framework

The Rook Test Framework is used to run end to end and integration tests on Rook. The framework depends on a running instance of Kubernetes.
The framework also provides scripts for starting Kubernetes so users can
quickly spin up a Kubernetes cluster. The Test framework is designed to install Rook, run tests, and uninstall Rook.

## Install Kubernetes

To install Kubernetes follow Rook's [developer guide](https://rook.io/docs/rook/latest/development-environment.html).

## Run Tests

### 1. Build rook

Now that the Kubernetes cluster is running we need to populate the Docker registry environment variables:

```console
eval $(minikube docker-env -p minikube)
```

The build process will build and push the images to the Docker registry **inside** the Minikube
virtual machine.

```console
make build
```

Now tag the newly built images to `rook/ceph:local-build` for running tests, or `rook/ceph:master` if creating sample manifests::

```console
docker tag $(docker images|awk '/build-/ {print $1}') rook/ceph:local-build
docker tag rook/ceph:local-build rook/ceph:master
```

### 2. Run integration tests

Some settings are available to run the tests under different environments. The settings are all configured with environment variables.
See [environment.go](/tests/framework/installer/environment.go) for the available environment variables.
At least you should set the following variables.

```console
export TEST_HELM_PATH=/tmp/rook-tests-scripts-helm/linux-amd64/helm
export TEST_BASE_DIR=WORKING_DIR
export TEST_SCRATCH_DEVICE=/dev/vdb
```

Please note that the integration tests erase the contents of TEST_SCRATCH_DEVICE.

To run all integration tests:

```console
go test -v -timeout 7200s github.com/rook/rook/tests/integration
```

After running tests, you can get test logs under "tests/integration/_output".

To run a specific suite (uses regex):

```console
go test -v -timeout 1800s -run CephSmokeSuite github.com/rook/rook/tests/integration
```

To run specific tests inside a suite:

```console
go test -v -timeout 1800s -run CephSmokeSuite github.com/rook/rook/tests/integration -testify.m TestARookClusterInstallation_SmokeTest
```

### 3. To run tests on OpenShift environment

- Setup OpenShift environment and export KUBECONFIG before executing the tests.
- Make sure `oc` executable file is in the PATH.
- Only `CephSmokeSuite` is currently supported on OpenShift.
- Set few environment variables:

```console
export TEST_ENV_NAME=openshift
export TEST_STORAGE_CLASS=gp2
export TEST_BASE_DIR=/tmp
export RETRY_MAX=40
```

To run the `CephSmokeSuite`:

```console
go test -v -timeout 1800s -run CephSmokeSuite github.com/rook/rook/tests/integration
```
