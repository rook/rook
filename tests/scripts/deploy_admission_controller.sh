#!/usr/bin/env bash
# deploy_admission_controller.sh
# Sets up the environment for the admission controller webhook in the active cluster.
set -eo pipefail

# Set our known directories and parameters.
BASE_DIR=$(cd "$(dirname "$0")"; pwd)

# Set the variables for webhook. Any change here for namespace and service name should be done in pkg/operator/admission/init.go also
[ -z "${NAMESPACE}" ] && NAMESPACE="rook-ceph"
WEBHOOK_CONFIG_NAME="rook-ceph-webhook"
SERVICE_NAME="rook-ceph-admission-controller"

INSTALL_SELF_SIGNED_CERT=true
echo "$BASE_DIR"

if [ "${INSTALL_SELF_SIGNED_CERT}" == true ]; then
	"${BASE_DIR}"/webhook-create-signed-cert.sh --namespace ${NAMESPACE}
fi
echo "Deploying webhook config"
export NAMESPACE
export WEBHOOK_CONFIG_NAME
export SERVICE_NAME
cat ${BASE_DIR}/webhook-config.yaml | \
        "${BASE_DIR}"/webhook-patch-ca-bundle.sh | \
        sed -e "s|\${NAMESPACE}|${NAMESPACE}|g" | \
        sed -e "s|\${WEBHOOK_CONFIG_NAME}|${WEBHOOK_CONFIG_NAME}|g" | \
        sed -e "s|\${SERVICE_NAME}|${SERVICE_NAME}|g" | \
        kubectl create -f -
echo "Webhook deployed! Please start the rook operator to create the service and admission controller pods"
