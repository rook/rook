---
title: Rook Test Framework
---

# Integration Tests

The integration tests run end-to-end tests on Rook in a running instance of Kubernetes.
The framework includes scripts for starting Kubernetes so users can
quickly spin up a Kubernetes cluster. The tests are generally designed to install Rook, run tests, and uninstall Rook.

The CI runs the integration tests with each PR and each master or release branch build.
If the tests fail in a PR, [access](development-flow.md#tmate-session) the tmate for debugging.

This document will outline the steps to run the integration tests locally in a minikube environment, should the CI
not be sufficient to troubleshoot.

!!! hint
    The CI is generally much simpler to troubleshoot than running these tests locally. Running the tests locally is rarely necessary.

!!! warning
    A risk of running the tests locally is that a local disk is required during the tests. If not running
    in a VM, your laptop or other test machine could be destroyed.

## Install Minikube

Follow Rook's [developer guide](https://rook.io/docs/rook/latest/Contributing/development-environment/) to install Minikube.

## Build Rook image

Now that the Kubernetes cluster is running we need to populate the Docker registry to allow local image builds
to be easily used inside Minikube.

```console
eval $(minikube docker-env -p minikube)
```

`make build` will now build and push the images to the Docker registry **inside** the Minikube
virtual machine.

```console
make build
```

Tag the newly built images to `rook/ceph:local-build` for running tests, or `rook/ceph:master` if creating example manifests::

```console
docker tag $(docker images|awk '/build-/ {print $1}') rook/ceph:local-build
docker tag rook/ceph:local-build rook/ceph:master
```

## Run integration tests

Some settings are available to run the tests under different environments. The settings are all configured with environment variables.
See [environment.go](/tests/framework/installer/environment.go) for the available environment variables.

Set the following variables:

```console
export TEST_HELM_PATH=/tmp/rook-tests-scripts-helm/linux-amd64/helm
export TEST_BASE_DIR=WORKING_DIR
export TEST_SCRATCH_DEVICE=/dev/vdb
```

Set `TEST_SCRATCH_DEVICE` to the correct block device name based on the driver that's being used.

!!! hint
    If using the `virtualbox` minikube driver, the device should be `/dev/sdb`

!!! warning
    The integration tests erase the contents of `TEST_SCRATCH_DEVICE` when the test is completed

To run a specific suite, specify the suite name:

```console
go test -v -timeout 1800s -run CephSmokeSuite github.com/rook/rook/tests/integration
```

After running tests, see test logs under `tests/integration/_output`.

To run specific tests inside a suite:

```console
go test -v -timeout 1800s -run CephSmokeSuite github.com/rook/rook/tests/integration -testify.m TestARookClusterInstallation_SmokeTest
```

!!! info
    Only the golang test suites are documented to run locally. Canary and other tests have only ever been supported in the CI.

## Running tests on OpenShift

1. Setup OpenShift environment and export KUBECONFIG
2. Make sure `oc` executable file is in the PATH.
3. Only the `CephSmokeSuite` is currently supported on OpenShift.
4. Set the following environment variables depending on the environment:

```console
export TEST_ENV_NAME=openshift
export TEST_STORAGE_CLASS=gp2
export TEST_BASE_DIR=/tmp
```

5. Run the integration tests
