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
  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
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
  scriptdir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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
  #enable kv engine v1 for osd and v2,transit for rgw encryption respectively in different path
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt -path=rook/ver1 kv
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt -path=rook/ver2 kv-v2
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt transit
  kubectl exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver1 || true # failure is expected
  kubectl exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver2 || true # failure is expected

  # Configure Vault Policy for Rook
  echo '
  path "rook/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
  }
  path "sys/mounts" {
  capabilities = ["read"]
  }
  path "transit/keys/*" {
    capabilities = [ "create", "update" ]
    denied_parameters = {"exportable" = [], "allow_plaintext_backup" = [] }
  }
  path "transit/keys/*" {
    capabilities = ["read", "delete"]
  }
  path "transit/keys/" {
    capabilities = ["list"]
  }
  path "transit/keys/+/rotate" {
    capabilities = [ "update" ]
  }
  path "transit/*" {
    capabilities = [ "update" ]
  }' | kubectl exec -i vault-0 -- vault policy write -ca-cert /vault/userconfig/vault-server-tls/vault.crt "$VAULT_POLICY_NAME" -

  # Configure Kubernetes auth
  if [[ "${KUBERNETES_AUTH}" == "true" ]]; then
    set_up_vault_kubernetes_auth
  else
    # Create a token for Rook
    ROOK_TOKEN=$(kubectl exec vault-0 -- vault token create -policy=rook -format json -ca-cert /vault/userconfig/vault-server-tls/vault.crt | jq -r '.auth.client_token' | base64)

    # Configure cluster
    sed -i "s|ROOK_TOKEN|${ROOK_TOKEN//[$'\t\r\n']/}|" tests/manifests/test-kms-vault.yaml
  fi
}

function validate_rgw_token {
  echo "wait for rgw pod to be ready"
  kubectl wait --for=condition=ready pod -l app=rook-ceph-rgw -n rook-ceph --timeout=100s
  RGW_POD=$(kubectl get pods -l app=rook-ceph-rgw -n rook-ceph --no-headers -o custom-columns=":metadata.name")
  RGW_TOKEN_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-token-file" | cut -f2- -d=)
  VAULT_PATH_PREFIX=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-prefix" | cut -f2- -d=)
  VAULT_TOKEN=$(kubectl -n rook-ceph exec $RGW_POD -- cat $RGW_TOKEN_FILE)
  VAULT_CACERT_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-cacert" | cut -f2- -d=)
  VAULT_CLIENT_CERT_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-clientcert" | cut -f2- -d=)
  VAULT_CLIENT_KEY_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-ssl-clientkey" | cut -f2- -d=)
  VAULT_SECRET_ENGINE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep "rgw-crypt-vault-secret-engine" | cut -f2- -d=)

  if [[ "$VAULT_SECRET_ENGINE" == "kv" ]]; then
    # Create secret for RGW server in kv engine
    ENCRYPTION_KEY=$(openssl rand -base64 32)
    kubectl exec vault-0 -- vault kv put -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver2/"$RGW_BUCKET_KEY" key="$ENCRYPTION_KEY"
    #fetch key from vault server using token from RGW pod
    FETCHED_KEY=$(kubectl -n rook-ceph exec $RGW_POD -- curl --key "$VAULT_CLIENT_KEY_FILE" --cert "$VAULT_CLIENT_CERT_FILE" --cacert "$VAULT_CACERT_FILE" -X GET -H "X-Vault-Token:$VAULT_TOKEN" "$VAULT_SERVER""$VAULT_PATH_PREFIX"/"$RGW_BUCKET_KEY" | jq -r .data.data.key)
    if [[ "$ENCRYPTION_KEY" != "$FETCHED_KEY" ]]; then
      echo "The set key $ENCRYPTION_KEY is different from fetched key $FETCHED_KEY"
      exit 1
    fi
  elif [[ "$VAULT_SECRET_ENGINE" == "transit" ]]; then
    # Create secret for RGW server in transit engine
    kubectl exec vault-0 -- vault write -ca-cert /vault/userconfig/vault-server-tls/vault.crt -f transit/keys/"$RGW_BUCKET_KEY"
    # check key exists via curl from RGW pod using credentials
    HTTP_STATUS=$(kubectl -n rook-ceph exec $RGW_POD -- curl -s -o /dev/null -w "%{http_code}" --key "$VAULT_CLIENT_KEY_FILE" --cert "$VAULT_CLIENT_CERT_FILE" --cacert "$VAULT_CACERT_FILE" -X PUT -H "X-Vault-Token:$VAULT_TOKEN" "$VAULT_SERVER""$VAULT_PATH_PREFIX"/datakey/plaintext/"$RGW_BUCKET_KEY")
    if [ "$HTTP_STATUS" -ne 200 ]; then
      echo "The http status code $HTTP_STATUS is different from 200"
      exit 1
    fi

  fi
}

function set_up_vault_kubernetes_auth {
  # create service account for vault to validate API token
  kubectl -n "$ROOK_NAMESPACE" create serviceaccount "$ROOK_VAULT_SA"

  # create the RBAC for this SA
  kubectl -n "$ROOK_NAMESPACE" create clusterrolebinding vault-tokenreview-binding --clusterrole=system:auth-delegator --serviceaccount="$ROOK_NAMESPACE":"$ROOK_VAULT_SA"
  # The service account generated a secret that is required for
  # configuration automatically in Kubernetes 1.23. In Kubernetes
  # 1.24+, we need to create the secret explicitly.
  kubectl apply -f - <<EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: rook-vault-auth-secret
  namespace: rook-ceph
  annotations:
    kubernetes.io/service-account.name: rook-vault-auth
type: kubernetes.io/service-account-token
EOF

  timeout 20 bash <<EOF
while ! kubectl --namespace rook-ceph get secret rook-vault-auth-secret >/dev/null 2>&1 ;do
  echo "Waiting for rook-vault-auth-secret secret.";
  sleep 1;
done
EOF

  # get the service account common.yaml created earlier
  VAULT_SA_SECRET_NAME=$(kubectl -n "$ROOK_NAMESPACE" get secrets --output=json | jq -r '.items[].metadata | select(.name|startswith("rook-vault-auth-")).name')
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
  NB_OSD_PVC=$(kubectl -n rook-ceph get pvc | grep -c set1)
  NB_VAULT_SECRET=$(kubectl -n default exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver1 | grep -c set1)

  if [ "$NB_OSD_PVC" -ne "$NB_VAULT_SECRET" ]; then
    echo "number of osd pvc is $NB_OSD_PVC and number of vault secret is $NB_VAULT_SECRET, mismatch"
    exit 1
  fi
}

function validate_key_rotation() {
  local backend_path=$1
  pvc_name=$(kubectl get pvc -n rook-ceph -l ceph.rook.io/setIndex=0 -o jsonpath='{.items[0].metadata.name}')
  key_name="rook-ceph-osd-encryption-key-$pvc_name"
  cmd="vault kv get -format=json $backend_path/$key_name"
  old_key=$(kubectl exec vault-0 -- sh -c "$cmd" | jq -r ".data.\"$key_name\"")
  local new_key
  runtime=180
  endtime=$((SECONDS + runtime))
  while [ $SECONDS -le $endtime ]; do
    echo "Time Now: $(date +%H:%M:%S)"
    new_key=$(kubectl exec vault-0 -- sh -c "$cmd" | jq -r ".data.\"$key_name\"")

    if [ "$old_key" != "$new_key" ]; then
      echo "encryption passphrase is successfully rotated"
      exit 0
    fi

    echo "encryption passphrase is not rotated, sleeping for 10 seconds"
    sleep 10
  done

  echo "encryption passphrase is not rotated"
  exit 1
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
validate_key_rotation)
  validate_key_rotation "$2"
  ;;
*)
  echo "invalid action $ACTION" >&2
  exit 1
  ;;
esac
