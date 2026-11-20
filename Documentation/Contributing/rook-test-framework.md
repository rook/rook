---
title: Rook Test Framework
---

# Integration Tests

The integration tests run end-to-end tests on Rook in a running instance of Kubernetes.
The framework includes scripts for starting Kubernetes so users can
quickly spin up a Kubernetes cluster. The tests are generally designed to install Rook, run tests, and uninstall Rook.

The CI runs the integration tests with each PR and each master or release branch build.
If the tests fail in a PR, [access](development-flow.md#ssh-session) the CI runner with ssh for debugging.

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
See [environment.go](https://github.com/rook/rook/blob/master/tests/framework/installer/environment.go) for the available environment variables.

Set the following variables:

```console
export TEST_HELM_PATH=/tmp/rook-tests-scripts-helm/helm
export TEST_BASE_DIR=WORKING_DIR
export TEST_SCRATCH_DEVICE=/dev/vdb
```

The `helm` tool is expected to be installed on the system. If it is not available in the `$PATH`, set the variable `TEST_HELM_PATH`.
otherwise, it should be set to the non-standard location of the  `helm` binary.

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

## Leak guard

From the install of Rook, the leak guard follows every container log and captures the pod specs in
the suite's namespaces, and in the namespaces the object tests create. It keeps the value of each key of
every Kubernetes Secret it sees, except short values and keys that are identifiers by design (see
below), and reads the cephx keys from the toolbox before the teardown.
After the uninstall it scans everything captured for those values (exact match) and for credential
patterns (with [betterleaks](https://github.com/betterleaks/betterleaks)).

An exact-match finding fails the test, unless `LEAK_GUARD_MODE=report` is set, which only reports
it. Pattern findings are only reported. The test fails in either mode if the guard captured no
container log, collected no secret from the cluster, found no cephx keys in the toolbox of a
namespace, or hit an error.

The evidence is under `tests/integration/_output/tests/leakguard-<test>/`, which CI uploads as the
job artifact on failure:

* `logs/`, `specs/` and `snapshots/` hold what was captured.
* `findings.txt` lists every finding.

The report in the test log groups the findings by secret and shows a few locations of each.
Neither the report nor `findings.txt` prints a secret value, only the name of the secret, or the
name of the rule for a pattern finding, and the `file:line` of the finding.

When the test had otherwise passed, a leak fails it after the uninstall, so the cluster is already
torn down and the usual failure-time log gathering finds nothing of it. When the test had already
failed, the uninstall skips the teardown and leaves the cluster as it is, and the guard still checks
what it captured. Either way the `leakguard-<test>/` directory is the evidence. A `go test -timeout`
that expires during the teardown loses the verdict.

To investigate a finding, open the named file at the named line and decide whether the value is
really a secret or an identifier. A value that is public by design, such as the cluster FSID, is
listed in `k8sSecretKeysSkipped` in
[sources.go](https://github.com/rook/rook/blob/master/tests/framework/leakguard/sources.go).
That list applies to the key in every Secret, so read its comment before adding to it.

## Assertions

The tests use [testify](https://github.com/stretchr/testify), which offers two families of checks.
`require` stops the current test immediately on failure. `assert` records the failure and keeps
going. Which one to reach for depends on what the check is doing:

* Use `require` for anything the rest of the test cannot proceed without: creating a resource,
    fetching the object that is about to be inspected, decoding a response, waiting for setup to
    finish. Continuing past one of these failures produces a nil dereference or a cascade of
    confusing errors that hide the original one.
* Use `assert` for the properties the test exists to verify, so that a single run reports every
    property that is wrong rather than only the first. Use it for cleanup as well, so one failed
    deletion does not strand the deletions after it.

Never call `assert` or `require` inside the closure of a poll or retry helper. A failed assertion
there defeats the retry, and testify's `Eventually` runs the closure on another goroutine, where
`require` cannot safely stop the test. Have the closure return false while the condition is unmet,
capture the value once it is met, and assert on it after the wait returns.

!!! note
    `FailNow`, and therefore `require`, only aborts the test goroutine it is called on. A `require`
    inside a `t.Run` closure stops that subtest, not its siblings or its parent.

## Running tests on OpenShift

1. Setup OpenShift environment and export KUBECONFIG
2. Make sure `oc` executable file is in the PATH.
3. Only the `CephSmokeSuite` is currently supported on OpenShift.
4. Set the following environment variables depending on the environment:

    ```console
    export TEST_ENV_NAME=openshift
    export TEST_STORAGE_CLASS=gp2-csi
    export TEST_BASE_DIR=/tmp
    ```

5. Run the integration tests
