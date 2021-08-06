#!/usr/bin/env bash
set -xe

DIR=$1
SERVICE=$2
NAMESPACE=$3
IP=$4
if [ -z "${IP}" ]; then
    IP=127.0.0.1
fi

openssl genrsa -out "${DIR}"/"${SERVICE}".key 2048

cat <<EOF >"${DIR}"/csr.conf
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
IP.1  = ${IP}
EOF

openssl req -new -key "${DIR}"/"${SERVICE}".key -subj "/CN=system:node:${SERVICE};/O=system:nodes" -out "${DIR}"/server.csr -config "${DIR}"/csr.conf

export CSR_NAME=${SERVICE}-csr

# Minimum 1.19.0 kubernetes version is required for certificates.k8s.io/v1 version
SERVER_VERSION=$(kubectl version --short | awk -F  "."  '/Server Version/ {print $2}')
MINIMUM_VERSION=19
if [ "${SERVER_VERSION}" -lt "${MINIMUM_VERSION}" ]
then
    cat <<EOF >"${DIR}"/csr.yaml
  apiVersion: certificates.k8s.io/v1beta1
  kind: CertificateSigningRequest
  metadata:
    name: ${CSR_NAME}
  spec:
    groups:
    - system:authenticated
    request: $(cat "${DIR}"/server.csr | base64 | tr -d '\n')
    usages:
    - digital signature
    - key encipherment
    - server auth
EOF
else
    cat <<EOF >"${DIR}"/csr.yaml
  apiVersion: certificates.k8s.io/v1
  kind: CertificateSigningRequest
  metadata:
    name: ${CSR_NAME}
  spec:
    groups:
    - system:authenticated
    request: $(cat "${DIR}"/server.csr | base64 | tr -d '\n')
    signerName: kubernetes.io/kubelet-serving
    usages:
    - digital signature
    - key encipherment
    - server auth
EOF
fi

kubectl create -f "${DIR}/"csr.yaml

kubectl certificate approve "${CSR_NAME}"

serverCert=$(kubectl get csr "${CSR_NAME}" -o jsonpath='{.status.certificate}')
echo "${serverCert}" | openssl base64 -d -A -out "${DIR}"/"${SERVICE}".crt
kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}' | base64 -d > "${DIR}"/"${SERVICE}".ca
