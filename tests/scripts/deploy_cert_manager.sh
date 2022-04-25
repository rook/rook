#!/usr/bin/env bash
# Sets up the cert-manager for CI
set -eEo pipefail

function error_log() {
  set +e -x
  kubectl -n rook-ceph get issuer
  kubectl -n rook-ceph get certificate
  kubectl -n rook-ceph get secret | grep rook-ceph-admission-controller
  kubectl -n rook-ceph get validatingwebhookconfigurations.admissionregistration.k8s.io
  kubectl describe validatingwebhookconfigurations.admissionregistration.k8s.io cert-manager-webhook
  kubectl describe validatingwebhookconfigurations.admissionregistration.k8s.io rook-ceph-webhook
  kubectl -n cert-manager logs deploy/cert-manager-webhook --tail=10
  kubectl -n cert-manager logs deploy/cert-manager-cainjector --tail=10
 set -e +x
}

trap error_log ERR

# Set our known directories and parameters.
BASE_DIR=$(cd "$(dirname "$0")"; pwd)
CERT_VERSION="v1.3.1"

[ -z "${NAMESPACE}" ] && NAMESPACE="rook-ceph"
export NAMESPACE
export WEBHOOK_CONFIG_NAME="rook-ceph-webhook"
export SERVICE_NAME="rook-ceph-admission-controller"

echo "$BASE_DIR"

echo "Deploying cert-manager"
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/$CERT_VERSION/cert-manager.yaml
timeout 150 bash <<-'EOF'
    until [ $(kubectl -n cert-manager get pods --field-selector=status.phase=Running | grep -c ^cert-) -eq 3 ]; do
      echo "waiting for cert-manager pods to be in running state"
      sleep 1
    done
EOF

timeout 20 bash <<-'EOF'
    until [ $(kubectl -n cert-manager get pods -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 3 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
    done
EOF

timeout 25 bash <<-'EOF'
    until [ $(kubectl get validatingwebhookconfigurations cert-manager-webhook -o jsonpath='{.webhooks[*].clientConfig.caBundle}' | wc -c) -gt 1 ]; do
      echo "waiting for caInjector to inject in caBundle for cert-manager validating webhook"
      sleep 1
    done
EOF

timeout 25 bash <<-'EOF'
    until [ $(kubectl get mutatingwebhookconfigurations cert-manager-webhook -o jsonpath='{.webhooks[*].clientConfig.caBundle}' | wc -c) -gt 1 ]; do
      echo "waiting for caInjector to inject in caBundle for cert-managers mutating webhook"
      sleep 1
    done
EOF

echo "Successfully deployed cert-manager"
