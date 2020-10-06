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

function generate_vault_tls_config {
  openssl genrsa -out "${TMPDIR}"/vault.key 2048

  cat <<EOF >"${TMPDIR}"/csr.conf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
DNS.4 = ${SERVICE}.${NAMESPACE}.svc.cluster.local
IP.1 = 127.0.0.1
EOF

  openssl req -new -key "${TMPDIR}"/vault.key -subj "/CN=${SERVICE}.${NAMESPACE}.svc" -out "${TMPDIR}"/server.csr -config "${TMPDIR}"/csr.conf

  export CSR_NAME=vault-csr

  cat <<EOF >"${TMPDIR}"/csr.yaml
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: ${CSR_NAME}
spec:
  groups:
  - system:authenticated
  request: $(cat ${TMPDIR}/server.csr | base64 | tr -d '\n')
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

  kubectl create -f "${TMPDIR}/"csr.yaml

  kubectl certificate approve ${CSR_NAME}

  serverCert=$(kubectl get csr ${CSR_NAME} -o jsonpath='{.status.certificate}')
  echo "${serverCert}" | openssl base64 -d -A -out "${TMPDIR}"/vault.crt
  kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}' | base64 -d > "${TMPDIR}"/vault.ca
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
  generate_vault_tls_config
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
  kubectl exec -ti vault-0 -- vault secrets enable -ca-cert /vault/userconfig/vault-server-tls/vault.crt -path=rook kv
  kubectl exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook || true # failure is expected

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
  sed -i "s|ROOK_TOKEN|$ROOK_TOKEN|" tests/manifests/test-kms-vault.yaml
}

function validate_deployment {
  validate_pvc_secret
}

function validate_pvc_secret {
  NB_OSD_PVC=$(kubectl -n rook-ceph get pvc|grep -c set1)
  NB_VAULT_SECRET=$(kubectl -n default exec -ti vault-0 -- vault kv list -ca-cert /vault/userconfig/vault-server-tls/vault.crt rook|grep -c set1)

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
  validate)
    validate_deployment
  ;;
  *)
    echo "invalid action $ACTION" >&2
    exit 1
  esac