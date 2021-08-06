#!/bin/bash
set -ex

: "${ACTION:=${1}}"

#############
# VARIABLES #
#############
SERVICE=vault
NAMESPACE=default
ROOK_NAMESPACE=rook-ceph
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
  }'| kubectl exec -i vault-0 -- vault policy write -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook -

  # Create a token for Rook
  ROOK_TOKEN=$(kubectl exec vault-0 -- vault token create -policy=rook -format json -ca-cert /vault/userconfig/vault-server-tls/vault.crt|jq -r '.auth.client_token'|base64)

  # Configure cluster
  sed -i "s|ROOK_TOKEN|${ROOK_TOKEN//[$'\t\r\n']}|" tests/manifests/test-kms-vault.yaml
}

function validate_rgw_token {
  # Create secret for RGW server in kv engine
  ENCRYPTION_KEY=$(openssl rand -base64 32)
  kubectl exec vault-0 -- vault kv put -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook/ver2/"$RGW_BUCKET_KEY" key="$ENCRYPTION_KEY"
  RGW_POD=$(kubectl -n rook-ceph get pods -l app=rook-ceph-rgw | awk 'FNR == 2 {print $1}')
  RGW_TOKEN_FILE=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep  "rgw-crypt-vault-token-file" | cut -f2- -d=)
  VAULT_PATH_PREFIX=$(kubectl -n rook-ceph describe pods "$RGW_POD" | grep  "rgw-crypt-vault-prefix" | cut -f2- -d=)
  VAULT_TOKEN=$(kubectl -n rook-ceph exec $RGW_POD -- cat $RGW_TOKEN_FILE)

  #fetch key from vault server using token from RGW pod, P.S using -k for curl since custom ssl certs not yet to support in RGW
  FETCHED_KEY=$(kubectl -n rook-ceph exec $RGW_POD -- curl -k -X GET -H "X-Vault-Token:$VAULT_TOKEN" "$VAULT_SERVER""$VAULT_PATH_PREFIX"/"$RGW_BUCKET_KEY"|jq -r .data.data.key)
  if [[ "$ENCRYPTION_KEY" != "$FETCHED_KEY" ]]; then
    echo "The set key $ENCRYPTION_KEY is different from fetched key $FETCHED_KEY"
    exit 1
  fi
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
