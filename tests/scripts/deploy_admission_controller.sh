#!/usr/bin/env bash
# deploy_admission_controller.sh
# Sets up the environment for the admission controller webhook in the active cluster.
set -eEo pipefail

function cleanup() {
  set +e
  kubectl -n rook-ceph delete validatingwebhookconfigurations $WEBHOOK_CONFIG_NAME
  kubectl -n rook-ceph delete certificate rook-admission-controller-cert
  kubectl -n rook-ceph delete issuers selfsigned-issuer
  kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/$CERT_VERSION/cert-manager.yaml
  set -e
}

function error_log() {
  set +e -x
  kubectl get validatingwebhookconfigurations cert-manager-webhook -o yaml | grep caBundle
  kubectl get mutatingwebhookconfigurations cert-manager-webhook -o yaml | grep caBundle
  kubectl -n rook-ceph get certificate
  kubectl -n rook-ceph get secret | grep rook-ceph-admission-controller
  kubectl -n rook-ceph get validatingwebhookconfigurations.admissionregistration.k8s.io 
  kubectl describe validatingwebhookconfigurations.admissionregistration.k8s.io cert-manager-webhook 
  kubectl describe validatingwebhookconfigurations.admissionregistration.k8s.io rook-ceph-webhook 
  kubectl -n cert-manager logs deploy/cert-manager-webhook --tail=10
  kubectl -n cert-manager logs deploy/cert-manager-cainjector --tail=10
 set -e +x
 cleanup
}

trap cleanup SIGINT 
trap error_log ERR

# Minimum 1.16.0 kubernetes version is required to start the admission controller
SERVER_VERSION=$(kubectl version --short | awk -F  "."  '/Server Version/ {print $2}')
MINIMUM_VERSION=16

if [ ${SERVER_VERSION} -lt ${MINIMUM_VERSION} ]; then
    echo "required minimum kubernetes version 1.$MINIMUM_VERSION.0"
    exit
fi

# Set our known directories and parameters.
BASE_DIR=$(cd "$(dirname "$0")"; pwd)
CERT_VERSION="v1.2.0"

[ -z "${NAMESPACE}" ] && NAMESPACE="rook-ceph"
export NAMESPACE
export WEBHOOK_CONFIG_NAME="rook-ceph-webhook"
export SERVICE_NAME="rook-ceph-admission-controller"

echo "$BASE_DIR"

echo "Deploying cert-manager"
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/$CERT_VERSION/cert-manager.yaml
timeout 150 sh -c 'until [ $(kubectl -n cert-manager get pods --field-selector=status.phase=Running|grep -c ^cert-) -eq 3 ]; do sleep 1; done'
timeout 20 sh -c 'until [ $(kubectl -n cert-manager get pods -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 3 ]; do sleep 1; done'
sleep 20

echo "Successfully deployed cert-manager"

echo "Creating Issuer and Certificate"
cat <<EOF | kubectl create -f -
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: ${NAMESPACE}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: rook-admission-controller-cert
  namespace: ${NAMESPACE}
spec:
  dnsNames:
  - ${SERVICE_NAME}
  - ${SERVICE_NAME}.${NAMESPACE}.svc
  - ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: selfsigned-issuer
  secretName: rook-ceph-admission-controller
EOF

echo "Successfully created Issuer and Certificate"

echo "Deploying webhook config"
cat ${BASE_DIR}/webhook-config.yaml | \
        "${BASE_DIR}"/webhook-patch-ca-bundle.sh | \
        sed -e "s|\${NAMESPACE}|${NAMESPACE}|g" | \
        sed -e "s|\${WEBHOOK_CONFIG_NAME}|${WEBHOOK_CONFIG_NAME}|g" | \
        sed -e "s|\${SERVICE_NAME}|${SERVICE_NAME}|g" | \
        kubectl create -f -
echo "Webhook deployed! Please start the rook operator to create the service and admission controller pods"
