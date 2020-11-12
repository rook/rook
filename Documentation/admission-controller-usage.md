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
1. Creates self-signed certificate.
1. Creates a Certificate Signing Request(CSR) for the certificate and gets it approved from the Kubernetes cluster.
1. Stores these certificates as a Kubernetes Secret.
1. Creates a Service Account, ClusterRole and ClusterRoleBindings for running the webhook server with minimal privileges.
1. Creates ValidatingWebhookConfig and fills the CA bundle with the appropriate value from the cluster.

Run the following commands:
```console
kubectl create -f examples/kubernetes/ceph/crds.yaml -f examples/kubernetes/ceph/common.yaml
cluster/examples/kubernetes/ceph/config-admission-controller.sh
```
Now that the Secrets have been deployed, we can deploy the operator:
```console
kubectl create -f operator.yaml
```

At this point the operator will start the admission controller Deployment automatically and the Webhook will start intercepting requests for Rook resources.

## Certificate Management

    The script file creates a self-signed Kubernetes approved certificate and deploys it as a secret onto the cluster. It is mandatory that the Secret is named "rook-ceph-admission-controller" because Rook will look for the secret with such name before starting the admission controller servers.

In the case of deploying from scratch, the script needs to be executed once without any modification, and certificates will be automatically created and deployed as a Secret.

The above approach of using self-signed certificates is discouraged as it would be the job of the owner to maintain the certificates. The recommended approach would be to use a proper certificate manager and get signed certificates from a known certificate authority. Once these are available, create the secrets using the following command:

```console
kubectl create secret generic rook-ceph-admission-controller \
        --from-file=key.pem=${PRIVATE_KEY_NAME}.pem \
        --from-file=cert.pem=${PUBLIC_KEY_NAME}.pem
```

Once the Secrets are in the cluster, we can modify the parameter `INSTALL_SELF_SIGNED_CERT` to `false` and execute these scripts to deploy the components. This modification is required only when Secrets are created but the components (ValidatingWebhookConfig, RBAC) are yet to be deployed.
