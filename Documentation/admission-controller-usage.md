---
title: Admission Controller
weight: 2030
indent: true
---

# Admission Controller

An admission controller intercepts requests to the Kubernetes API server prior to persistence of the object, but after the request is authenticated and authorized.

Enabling the Rook admission controller is recommended to provide an additional level of validation that Rook is configured correctly with the custom resource (CR) settings.

## Quick Start

To deploy the Rook admission controllers we have a helper script that will automate the configuration.

This script will help us achieve the following tasks
1. Creates certificate using cert-manager.
2. Creates ValidatingWebhookConfig and fills the CA bundle with the appropriate value from the cluster.

Run the following commands:

```console
kubectl create -f deploy/examples/crds.yaml -f deploy/examples/common.yaml
tests/scripts/deploy_admission_controller.sh
```
Now that the Secrets have been deployed, we can deploy the operator:
```console
kubectl create -f deploy/examples/operator.yaml
```

At this point the operator will start the admission controller Deployment automatically and the Webhook will start intercepting requests for Rook resources.
