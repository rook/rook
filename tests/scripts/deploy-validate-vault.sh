#!/usr/bin/env bash

# Copyright 2021 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -exEuo pipefail

: "${ACTION:=${1}}"
: "${KUBERNETES_AUTH:=false}"

#############
# VARIABLES #
#############
SERVICE=vault
NAMESPACE=default
ROOK_NAMESPACE=rook-ceph
ROOK_VAULT_SA=rook-vault-auth
ROOK_SYSTEM_SA=rook-ceph-system
ROOK_OSD_SA=rook-ceph-osd
VAULT_POLICY_NAME=rook
SECRET_NAME=vault-server-tls
TMPDIR=$(mktemp -d)
VAULT_SERVER=https://vault.default:8200
RGW_BUCKET_KEY=mybucketkey
#############
# FUNCTIONS #
#############

function install_helm {
  curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
  sudo apt-get install apt-transport-https --yes
  echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
  sudo apt-get update
  sudo apt-get install helm
}

if [[ "$(uname)" == "Linux" ]]; then
  sudo apt-get install jq -y
  install_helm
fi

function create_secret_generic {
  kubectl create secret generic ${SECRET_NAME} \
  --namespace ${NAMESPACE} \
  --from-file=vault.key="${TMPDIR}"/vault.key \
  --from-file=vault.crt="${TMPDIR}"/vault.crt \
  --from-file=vault.ca="${TMPDIR}"/vault.ca

  # for rook
  kubectl create secret generic vault-ca-cert --namespace ${ROOK_NAMESPACE} --from-file=cert="${TMPDIR}"/vault.ca
  kubectl create secret generic vault-client-cert --namespace ${ROOK_NAMESPACE} --from-file=cert="${TMPDIR}"/vault.crt
  kubectl create secret generic vault-client-key --namespace ${ROOK_NAMESPACE} --from-file=key="${TMPDIR}"/vault.key
}

function vault_helm_tls {

cat <<EOF >"${TMPDIR}/"custom-values.yaml
global:
  enabled: true
  tlsDisable: false

server:
  extraEnvironmentVars:
    VAULT_CACERT: /vault/userconfig/vault-server-tls/vault.ca

  extraVolumes:
  - type: secret
    name: vault-server-tls # Matches the ${SECRET_NAME} from above

  standalone:
    enabled: true
    config: |
      listener "tcp" {
        address = "[::]:8200"
        cluster_address = "[::]:8201"
        tls_cert_file = "/vault/userconfig/vault-server-tls/vault.crt"
        tls_key_file  = "/vault/userconfig/vault-server-tls/vault.key"
        tls_client_ca_file = "/vault/userconfig/vault-server-tls/vault.ca"
      }

      storage "file" {
        path = "/vault/data"
      }
EOF

}

function deploy_vault {
  # TLS config
  scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  bash "${scriptdir}"/generate-tls-config.sh "${TMPDIR}" ${SERVICE} ${NAMESPACE}
  create_secret_generic
  vault_helm_tls

  # Install Vault with Helm
  helm repo add hashicorp https://helm.releases.hashicorp.com
  helm install vault hashicorp/vault --values "${TMPDIR}/"custom-values.yaml
  timeout 120 sh -c 'until kubectl get pods -l app.kubernetes.io/name=vault --field-selector=status.phase=Running|grep vault-0; do sleep 5; done'

  # Unseal Vault
  VAULT_INIT_TEMP_DIR=$(mktemp)
  kubectl exec -ti vault-0 -- vault operator init -format "json" -ca-cert /vault/userconfig/vault-server-tls/vault.crt | tee -a "$VAULT_INIT_TEMP_DIR"
  for i in $(seq 0 2); do
    kubectl exec -ti vault-0 -- vault operator unseal -ca-cert /vault/userconfig/vault-server-tls/vault.crt "$(jq -r ".unseal_keys_b64[$i]" "$VAULT_INIT_TEMP_DIR")"
  done
  kubectl get pods -l app.kubernetes.io/name=vault

  # Wait for vault to be ready once unsealed
  while [[ $(kubectl get pods -l app.kubernetes.io/name=vault -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do echo "waiting vault to be ready" && sleep 1; done

  # Configure Vault
  ROOT_TOKEN=$(jq -r '.root_token' "$VAULT_INIT_TEMP_DIR")
  kubectl exec -it vault-0 -- vault login -ca-cert /vault/userconfig/vault-server-tls/vault.crt "$ROOT_TOKEN"
  #enable kv engine v1 for osd and v2 for rgw encryption respectively in different path
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt -path=rook/ver1 kv
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt -path=rook/ver2 kv-v2
  kubectl exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver1 || true # failure is expected
  kubectl exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver2 || true # failure is expected

  # Configure Vault Policy for Rook
  echo '
  path "rook/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
  }
  path "sys/mounts" {
  capabilities = ["read"]
  }'| kubectl exec -i vault-0 -- vault policy write -ca-cert /vault/userconfig/vault-server-tls/vault.crt "$VAULT_POLICY_NAME" -

  # Configure Kubernetes auth
  if [[ "${KUBERNETES_AUTH}" == "true" ]]; then
    set_up_vault_kubernetes_auth
  else
    # Create a token for Rook
    ROOK_TOKEN=$(kubectl exec vault-0 -- vault token create -policy=rook -format json -ca-cert /vault/userconfig/vault-server-tls/vault.crt|jq -r '.auth.client_token'|base64)

    # Configure cluster
    sed -i "s|ROOK_TOKEN|${ROOK_TOKEN//[$'\t\r\n']}|" tests/manifests/test-kms-vault.yaml
  fi
}

function validate_rgw_token {
  # Create secret for RGW server in kv engine
  ENCRYPTION_KEY=$(openssl rand -base64 32)
  kubectl exec vault-0 -- vault kv put -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver2/"$RGW_BUCKET_KEY" key="$ENCRYPTION_KEY"
  RGW_POD=$(kubectl -n rook-ceph get pods -l app=rook-ceph-rgw | awk 'FNR == 2 {print $1}')
  RGW_TOKEN_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-token-file" | cut -f2- -d=)
  VAULT_PATH_PREFIX=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-prefix" | cut -f2- -d=)
  VAULT_TOKEN=$(kubectl -n rook-ceph exec $RGW_POD -- cat $RGW_TOKEN_FILE)
  VAULT_CACERT_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-cacert" | cut -f2- -d=)
  VAULT_CLIENT_CERT_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-clientcert" | cut -f2- -d=)
  VAULT_CLIENT_KEY_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-clientkey" | cut -f2- -d=)


  #fetch key from vault server using token from RGW pod, P.S using -k for curl since custom ssl certs not yet to support in RGW
  FETCHED_KEY=$(kubectl -n rook-ceph exec $RGW_POD -- curl --key "$VAULT_CLIENT_KEY_FILE" --cert "$VAULT_CLIENT_CERT_FILE" --cacert "$VAULT_CACERT_FILE" -X GET -H "X-Vault-Token:$VAULT_TOKEN" "$VAULT_SERVER""$VAULT_PATH_PREFIX"/"$RGW_BUCKET_KEY"|jq -r .data.data.key)
  if [[ "$ENCRYPTION_KEY" != "$FETCHED_KEY" ]]; then
    echo "The set key $ENCRYPTION_KEY is different from fetched key $FETCHED_KEY"
    exit 1
  fi
}

function set_up_vault_kubernetes_auth {
  # create service account for vault to validate API token
  kubectl -n "$ROOK_NAMESPACE" create serviceaccount "$ROOK_VAULT_SA"

  # create the RBAC for this SA
  kubectl -n "$ROOK_NAMESPACE" create clusterrolebinding vault-tokenreview-binding --clusterrole=system:auth-delegator --serviceaccount="$ROOK_NAMESPACE":"$ROOK_VAULT_SA"

  # get the service account common.yaml created earlier
  VAULT_SA_SECRET_NAME=$(kubectl -n "$ROOK_NAMESPACE" get sa "$ROOK_VAULT_SA" -o jsonpath="{.secrets[*]['name']}")

  # Set SA_JWT_TOKEN value to the service account JWT used to access the TokenReview API
  SA_JWT_TOKEN=$(kubectl -n "$ROOK_NAMESPACE" get secret "$VAULT_SA_SECRET_NAME" -o jsonpath="{.data.token}" | base64 --decode)

  # Set SA_CA_CRT to the PEM encoded CA cert used to talk to Kubernetes API
  SA_CA_CRT=$(kubectl -n "$ROOK_NAMESPACE" get secret "$VAULT_SA_SECRET_NAME" -o jsonpath="{.data['ca\.crt']}" | base64 --decode)

  # get kubernetes endpoint
  K8S_HOST=$(kubectl config view --minify --flatten -o jsonpath="{.clusters[0].cluster.server}")

  # enable kubernetes auth
  kubectl exec -ti vault-0 -- vault auth enable kubernetes

  # configure the kubernetes auth
  kubectl exec -ti vault-0 -- vault write auth/kubernetes/config \
    token_reviewer_jwt="$SA_JWT_TOKEN" \
    kubernetes_host="$K8S_HOST" \
    kubernetes_ca_cert="$SA_CA_CRT" \
    issuer="https://kubernetes.default.svc.cluster.local"

  # configure a role for rook
  kubectl exec -ti vault-0 -- vault write auth/kubernetes/role/"$ROOK_NAMESPACE" \
    bound_service_account_names="$ROOK_SYSTEM_SA","$ROOK_OSD_SA" \
    bound_service_account_namespaces="$ROOK_NAMESPACE" \
    policies="$VAULT_POLICY_NAME" \
    ttl=1440h
}

function validate_osd_deployment {
  validate_osd_secret
}

function validate_rgw_deployment {
  validate_rgw_token
}

function validate_osd_secret {
  NB_OSD_PVC=$(kubectl -n rook-ceph get pvc|grep -c set1)
  NB_VAULT_SECRET=$(kubectl -n default exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver1|grep -c set1)

  if [ "$NB_OSD_PVC" -ne "$NB_VAULT_SECRET" ]; then
    echo "number of osd pvc is $NB_OSD_PVC and number of vault secret is $NB_VAULT_SECRET, mismatch"
    exit 1
  fi
}

########
# MAIN #
########

case "$ACTION" in
  deploy)
    deploy_vault
  ;;
  validate_osd)
    validate_osd_deployment
  ;;
  validate_rgw)
    validate_rgw_deployment
  ;;
  *)
    echo "invalid action $ACTION" >&2
    exit 1
esac
