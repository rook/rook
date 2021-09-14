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

set -xe

#############
# VARIABLES #
#############
: "${BLOCK:=$(sudo lsblk --paths | awk '/14G/ {print $1}' | head -1)}"
NETWORK_ERROR="connection reset by peer"
SERVICE_UNAVAILABLE_ERROR="Service Unavailable"
INTERNAL_ERROR="INTERNAL_ERROR"

#############
# FUNCTIONS #
#############

function install_deps() {
  sudo wget https://github.com/mikefarah/yq/releases/download/3.4.1/yq_linux_amd64 -O /usr/local/bin/yq
  sudo chmod +x /usr/local/bin/yq
}

function print_k8s_cluster_status() {
  kubectl cluster-info
  kubectl get pods -n kube-system
}

function use_local_disk() {
  BLOCK_DATA_PART=${BLOCK}1
  sudo dmsetup version || true
  sudo swapoff --all --verbose
  if mountpoint -q /mnt; then
    sudo umount /mnt
    # search for the device since it keeps changing between sda and sdb
    sudo wipefs --all --force "$BLOCK_DATA_PART"
  else
    # it's the hosted runner!
    sudo sgdisk --zap-all --clear --mbrtogpt -g -- "${BLOCK}"
    sudo dd if=/dev/zero of="${BLOCK}" bs=1M count=10 oflag=direct
    sudo parted -s "${BLOCK}" mklabel gpt
  fi
  sudo lsblk
}

function use_local_disk_for_integration_test() {
  sudo swapoff --all --verbose
  sudo umount /mnt
  # search for the device since it keeps changing between sda and sdb
  PARTITION="${BLOCK}1"
  sudo wipefs --all --force "$PARTITION"
  sudo lsblk
  # add a udev rule to force the disk partitions to ceph
  # we have observed that some runners keep detaching/re-attaching the additional disk overriding the permissions to the default root:disk
  # for more details see: https://github.com/rook/rook/issues/7405
  echo "SUBSYSTEM==\"block\", ATTR{size}==\"29356032\", ACTION==\"add\", RUN+=\"/bin/chown 167:167 $PARTITION\"" | sudo tee -a /etc/udev/rules.d/01-rook.rules
}

function create_partitions_for_osds() {
  tests/scripts/create-bluestore-partitions.sh --disk "$BLOCK" --osd-count 2
  sudo lsblk
}

function create_bluestore_partitions_and_pvcs() {
  BLOCK_PART="$BLOCK"2
  DB_PART="$BLOCK"1
  tests/scripts/create-bluestore-partitions.sh --disk "$BLOCK" --bluestore-type block.db --osd-count 1
  tests/scripts/localPathPV.sh "$BLOCK_PART" "$DB_PART"
}

function create_bluestore_partitions_and_pvcs_for_wal(){
  BLOCK_PART="$BLOCK"3
  DB_PART="$BLOCK"1
  WAL_PART="$BLOCK"2
  tests/scripts/create-bluestore-partitions.sh --disk "$BLOCK" --bluestore-type block.wal --osd-count 1
  tests/scripts/localPathPV.sh "$BLOCK_PART" "$DB_PART" "$WAL_PART"
}

function build_rook() {
  build_type=build
  if [ -n "$1" ]; then
    build_type=$1
  fi
  GOPATH=$(go env GOPATH) make clean
  for _ in $(seq 1 3); do
    if ! o=$(make -j"$(nproc)" IMAGES='ceph' "$build_type"); then
      case "$o" in
        *"$NETWORK_ERROR"*)
          echo "network failure occurred, retrying..."
          continue
        ;;
        *"$SERVICE_UNAVAILABLE_ERROR"*)
          echo "network failure occurred, retrying..."
          continue
        ;;
        *"$INTERNAL_ERROR"*)
          echo "network failure occurred, retrying..."
          continue
        ;;
        *)
          # valid failure
          exit 1
      esac
      # no errors so we break the loop after the first iteration
      break
    fi
  done
  # validate build
  tests/scripts/validate_modified_files.sh build
  docker images
  if [[ "$build_type" == "build" ]]; then
    docker tag $(docker images | awk '/build-/ {print $1}') rook/ceph:v1.7.3
  fi
}

function build_rook_all() {
  build_rook build.all
}

function validate_yaml() {
  cd cluster/examples/kubernetes/ceph
  kubectl create -f crds.yaml -f common.yaml
  # skipping folders and some yamls that are only for openshift.
  manifests="$(find . -maxdepth 1 -type f -name '*.yaml' -and -not -name '*openshift*' -and -not -name 'scc.yaml')"
  with_f_arg="$(echo "$manifests" | awk '{printf " -f %s",$1}')" # don't add newline
  kubectl create ${with_f_arg} --dry-run=client
}

function create_cluster_prerequisites() {
  cd cluster/examples/kubernetes/ceph
  kubectl create -f crds.yaml -f common.yaml
}

function deploy_cluster() {
  cd cluster/examples/kubernetes/ceph
  kubectl create -f operator.yaml
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\/}|g" cluster-test.yaml
  kubectl create -f cluster-test.yaml
  kubectl create -f object-test.yaml
  kubectl create -f pool-test.yaml
  kubectl create -f filesystem-test.yaml
  kubectl create -f rbdmirror.yaml
  kubectl create -f filesystem-mirror.yaml
  kubectl create -f nfs-test.yaml
  kubectl create -f toolbox.yaml
}

function wait_for_prepare_pod() {
  timeout 180 sh -c 'until kubectl -n rook-ceph logs -f job/$(kubectl -n rook-ceph get job -l app=rook-ceph-osd-prepare -o jsonpath='{.items[0].metadata.name}'); do sleep 5; done' || true
  timeout 60 sh -c 'until kubectl -n rook-ceph logs $(kubectl -n rook-ceph get pod -l app=rook-ceph-osd,ceph_daemon_id=0 -o jsonpath='{.items[*].metadata.name}') --all-containers; do echo "waiting for osd container" && sleep 1; done' || true
  kubectl -n rook-ceph describe job/$(kubectl -n rook-ceph get pod -l app=rook-ceph-osd-prepare -o jsonpath='{.items[*].metadata.name}') || true
  kubectl -n rook-ceph describe deploy/rook-ceph-osd-0 || true
}

function wait_for_ceph_to_be_ready() {
  DAEMONS=$1
  OSD_COUNT=$2
  mkdir test
  tests/scripts/validate_cluster.sh $DAEMONS $OSD_COUNT
  kubectl -n rook-ceph get pods
}

function check_ownerreferences() {
  curl -L https://github.com/kubernetes-sigs/kubectl-check-ownerreferences/releases/download/v0.2.0/kubectl-check-ownerreferences-linux-amd64.tar.gz -o kubectl-check-ownerreferences-linux-amd64.tar.gz
  tar xzvf kubectl-check-ownerreferences-linux-amd64.tar.gz
  chmod +x kubectl-check-ownerreferences
  ./kubectl-check-ownerreferences -n rook-ceph
}

function create_LV_on_disk() {
  sudo sgdisk --zap-all "${BLOCK}"
  VG=test-rook-vg
  LV=test-rook-lv
  sudo pvcreate "$BLOCK"
  sudo vgcreate "$VG" "$BLOCK" || sudo vgcreate "$VG" "$BLOCK" || sudo vgcreate "$VG" "$BLOCK"
  sudo lvcreate -l 100%FREE -n "${LV}" "${VG}"
  tests/scripts/localPathPV.sh /dev/"${VG}"/${LV}
  kubectl create -f cluster/examples/kubernetes/ceph/crds.yaml
  kubectl create -f cluster/examples/kubernetes/ceph/common.yaml
}

function generate_tls_config {
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

  openssl req -new -key "${DIR}"/"${SERVICE}".key -subj "/CN=${SERVICE}.${NAMESPACE}.svc" -out "${DIR}"/server.csr -config "${DIR}"/csr.conf

  export CSR_NAME=${SERVICE}-csr

  cat <<EOF >"${DIR}"/csr.yaml
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: ${CSR_NAME}
spec:
  groups:
  - system:authenticated
  request: $(cat ${DIR}/server.csr | base64 | tr -d '\n')
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

  kubectl create -f "${DIR}/"csr.yaml

  kubectl certificate approve ${CSR_NAME}

  serverCert=$(kubectl get csr ${CSR_NAME} -o jsonpath='{.status.certificate}')
  echo "${serverCert}" | openssl base64 -d -A -out "${DIR}"/"${SERVICE}".crt
  kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}' | base64 -d > "${DIR}"/"${SERVICE}".ca
}

selected_function="$1"
if [ "$selected_function" = "generate_tls_config" ]; then
    $selected_function $2 $3 $4 $5
elif [ "$selected_function" = "wait_for_ceph_to_be_ready" ]; then
     $selected_function $2 $3
else
  $selected_function
fi

if [ $? -ne 0 ]; then
  echo "Function call to '$selected_function' was not successful" >&2
  exit 1
fi
