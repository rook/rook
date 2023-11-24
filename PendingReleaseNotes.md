# v1.13 Pending Release Notes

## Breaking Changes

- Removed support for Ceph Pacific (v16)
- Support for the admission controller/webhooks has been removed. If admission controller/webhooks is enabled, disable by changing
`ROOK_DISABLE_ADMISSION_CONTROLLER: "true"` in operator.yaml before upgrading to rook v1.13. CRD validation is now enabled with [Common Expression Language](https://kubernetes.io/docs/reference/using-api/cel/). This requires Kubernetes version 1.25 or higher.

## Features
