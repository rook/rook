---
title: CI Configuration
---

This page contains information regarding the CI configuration used for the Rook project to test, build and release the project.

## Secrets

* Snyk (Security Scan):
    * `SNYK_TOKEN` - API Token for the [snyk security scanner](https://snyk.io/) (workflow file: `snyk.yaml`).
* Testing:
    * `IBM_INSTANCE_ID`: Used for KMS (Key Management System) IBM Key Protect access (see [`.github/workflows/encryption-pvc-kms-ibm-kp/action.yml`](https://github.com/rook/rook/blob/master/.github/workflows/encryption-pvc-kms-ibm-kp/action.yml)).
    * `IBM_SERVICE_API_KEY`: Used for KMS (Key Management System) IBM Key Protect access (see [`.github/workflows/encryption-pvc-kms-ibm-kp/action.yml`](https://github.com/rook/rook/blob/master/.github/workflows/encryption-pvc-kms-ibm-kp/action.yml)).
* Publishing:
    * `DOCKER_USERNAME` + `DOCKER_PASSWORD`: Username and password of registry.
    * `DOCKER_REGISTRY`: Target registry namespace (e.g., `rook`)
    * `AWS_USR` + `AWS_PSW`: AWS credentials with access to S3 for Helm chart publishing.
    * `GIT_API_TOKEN`: GitHub access token, used to push docs changes to the docs repositories `gh-pages` branch.

## SSH debugging

See [Debugging CI over SSH](debugging-ci-over-ssh.md). A debug session is always
restricted to the run's triggering actor (who must have write access to request
one), so no repository configuration is required. A self-hosted `uptermd` relay
can optionally be used via the `upterm-server` input of the
[`ci-ssh-debug`](https://github.com/rook/rook/blob/master/.github/workflows/ci-ssh-debug/action.yml)
action.
