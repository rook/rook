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

set -xeEo pipefail

#############
# VARIABLES #
#############
: "${BLOCK:=$(sudo lsblk --paths | awk '/14G/ || /64G/ {print $1}' | head -1)}"
NETWORK_ERROR="connection reset by peer"
SERVICE_UNAVAILABLE_ERROR="Service Unavailable"
INTERNAL_ERROR="INTERNAL_ERROR"
INTERNAL_SERVER_ERROR="500 Internal Server Error"

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

function prepare_loop_devices() {
  if [ $# -ne 1 ]; then
    echo "usage: $0 loop_deivce_count"
    exit 1
  fi
  OSD_COUNT=$1
  if [ $OSD_COUNT -le 0 ]; then
    echo "Invalid OSD_COUNT $OSD_COUNT. OSD_COUNT must be larger than 0."
    exit 1
  fi
  for i in $(seq 1 $OSD_COUNT); do
    sudo dd if=/dev/zero of=~/data${i}.img bs=1M seek=6144 count=0
    sudo losetup /dev/loop${i} ~/data${i}.img
  done
  sudo lsblk
}

function use_local_disk() {
  BLOCK_DATA_PART=${BLOCK}1
  sudo apt purge snapd -y
  sudo dmsetup version || true
  sudo swapoff --all --verbose
  if mountpoint -q /mnt; then
    sudo umount /mnt
    # search for the device since it keeps changing between sda and sdb
    sudo wipefs --all --force "$BLOCK_DATA_PART"
  else
    # it's the hosted runner!
    sudo sgdisk --zap-all -- "${BLOCK}"
    sudo dd if=/dev/zero of="${BLOCK}" bs=1M count=10 oflag=direct,dsync
    sudo parted -s "${BLOCK}" mklabel gpt
  fi
  sudo lsblk
}

function use_local_disk_for_integration_test() {
  sudo apt purge snapd -y
  sudo udevadm control --log-priority=debug
  sudo swapoff --all --verbose
  sudo umount /mnt
  sudo sed -i.bak '/\/mnt/d' /etc/fstab
  # search for the device since it keeps changing between sda and sdb
  PARTITION="${BLOCK}1"
  sudo wipefs --all --force "$PARTITION"
  sudo dd if=/dev/zero of="${PARTITION}" bs=1M count=1
  sudo lsblk --bytes
  # add a udev rule to force the disk partitions to ceph
  # we have observed that some runners keep detaching/re-attaching the additional disk overriding the permissions to the default root:disk
  # for more details see: https://github.com/rook/rook/issues/7405
  echo "SUBSYSTEM==\"block\", ATTR{size}==\"29356032\", ACTION==\"add\", RUN+=\"/bin/chown 167:167 $PARTITION\"" | sudo tee -a /etc/udev/rules.d/01-rook.rules
  # for below, see: https://access.redhat.com/solutions/1465913
  block_base="$(basename "${BLOCK}")"
  echo "ACTION==\"add|change\", KERNEL==\"${block_base}\", OPTIONS:=\"nowatch\"" | sudo tee -a /etc/udev/rules.d/99-z-rook-nowatch.rules
  # The partition is still getting reloaded occasionally during operation. See https://github.com/rook/rook/issues/8975
  # Try issuing some disk-inspection commands to jog the system so it won't reload the partitions
  # during OSD provisioning.
  sudo udevadm control --reload-rules || true
  sudo udevadm trigger || true
  time sudo udevadm settle || true
  sudo partprobe || true
  sudo lsblk --noheadings --pairs "${BLOCK}" || true
  sudo sgdisk --print "${BLOCK}" || true
  sudo udevadm info --query=property "${BLOCK}" || true
  sudo lsblk --noheadings --pairs "${PARTITION}" || true
  journalctl -o short-precise --dmesg | tail -40 || true
  cat /etc/fstab || true
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

function create_bluestore_partitions_and_pvcs_for_wal() {
  BLOCK_PART="$BLOCK"3
  DB_PART="$BLOCK"1
  WAL_PART="$BLOCK"2
  tests/scripts/create-bluestore-partitions.sh --disk "$BLOCK" --bluestore-type block.wal --osd-count 1
  tests/scripts/localPathPV.sh "$BLOCK_PART" "$DB_PART" "$WAL_PART"
}

function collect_udev_logs_in_background() {
  local log_dir="${1:-"/home/runner/work/rook/rook/tests/integration/_output/tests"}"
  mkdir -p "${log_dir}"
  udevadm monitor --property &>"${log_dir}"/udev-monitor-property.txt &
  udevadm monitor --kernel &>"${log_dir}"/udev-monitor-kernel.txt &
  udevadm monitor --udev &>"${log_dir}"/udev-monitor-udev.txt &
}

function check_empty_file() {
  output_file=$1
  if [ -s "$output_file" ]; then
    echo "script failed with stderr error"
    cat "$output_file"
    rm -f "$output_file"
    exit 1
  fi
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
      *"$INTERNAL_SERVER_ERROR"*)
        echo "network failure occurred, retrying..."
        continue
        ;;
      *)
        # valid failure
        echo "failed with the following log:"
        echo "$o"
        exit 1
        ;;
      esac
    fi
    # no errors so we break the loop after the first iteration
    break
  done
  # validate build
  tests/scripts/validate_modified_files.sh build
  docker images
  if [[ "$build_type" == "build" ]]; then
    docker tag "$(docker images | awk '/build-/ {print $1}')" rook/ceph:local-build
  fi
}

function build_rook_all() {
  build_rook build.all
}

function validate_yaml() {
  cd deploy/examples
  kubectl create -f crds.yaml -f common.yaml -f csi/nfs/rbac.yaml

  # create the volume replication CRDs
  replication_version=v0.3.0
  replication_url="https://raw.githubusercontent.com/csi-addons/volume-replication-operator/${replication_version}/config/crd/bases"
  kubectl create -f "${replication_url}/replication.storage.openshift.io_volumereplications.yaml"
  kubectl create -f "${replication_url}/replication.storage.openshift.io_volumereplicationclasses.yaml"

  #create the KEDA CRDS
  keda_version=2.4.0
  keda_url="https://github.com/kedacore/keda/releases/download/v${keda_version}/keda-${keda_version}.yaml"
  kubectl apply -f "${keda_url}"

  #create the COSI CRDS
  cosi_crd_url="github.com/kubernetes-sigs/container-object-storage-interface-api"
  kubectl create -k "${cosi_crd_url}"

  # skipping folders and some yamls that are only for openshift.
  manifests="$(find . -maxdepth 1 -type f -name '*.yaml' -and -not -name '*openshift*' -and -not -name 'scc*' -and -not -name 'psp*' -and -not -name 'kustomization*')"
  with_f_arg="$(echo "$manifests" | awk '{printf " -f %s",$1}')" # don't add newline
  # shellcheck disable=SC2086 # '-f manifest1.yaml -f manifest2.yaml etc.' should not be quoted
  kubectl create ${with_f_arg} --dry-run=client
}

function create_cluster_prerequisites() {
  # this might be called from another function that has already done a cd
  (cd deploy/examples && kubectl create -f crds.yaml -f common.yaml -f csi/nfs/rbac.yaml)
}

function deploy_manifest_with_local_build() {
  sed -i 's/.*ROOK_CSI_ENABLE_NFS:.*/  ROOK_CSI_ENABLE_NFS: \"true\"/g' $1
  if [[ "$USE_LOCAL_BUILD" != "false" ]]; then
    sed -i "s|image: rook/ceph:.*|image: rook/ceph:local-build|g" $1
  fi
  if [[ "$ALLOW_LOOP_DEVICES" = "true" ]]; then
    sed -i "s|ROOK_CEPH_ALLOW_LOOP_DEVICES: \"false\"|ROOK_CEPH_ALLOW_LOOP_DEVICES: \"true\"|g" $1
  fi
  sed -i "s|ROOK_LOG_LEVEL:.*|ROOK_LOG_LEVEL: DEBUG|g" "$1"
  kubectl create -f $1
}

# Deploy toolbox with same ceph version as the cluster-test for ci
function deploy_toolbox() {
  sed -i 's/image: quay\.io\/ceph\/ceph:.*/image: quay.io\/ceph\/ceph:v18/' toolbox.yaml
  kubectl create -f toolbox.yaml
}

function replace_ceph_image() {
  local file="$1"           # parameter 1: the file in which to replace the ceph image
  local ceph_image="${2:-}" # parameter 2: the new ceph image to use
  if [[ -z ${ceph_image} ]]; then
    echo "No Ceph image given. Not adjusting manifests."
    return 0
  fi
  sed -i "s|image: .*ceph/ceph:.*|image: ${ceph_image}|g" "${file}"
}

function deploy_cluster() {
  cd deploy/examples
  deploy_manifest_with_local_build operator.yaml
  if [ $# == 0 ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  elif [ "$1" = "two_osds_in_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}\n    config:\n      osdsPerDevice: \"2\"|g" cluster-test.yaml
  elif [ "$1" = "osd_with_metadata_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}\n    config:\n      metadataDevice: /dev/test-rook-vg/test-rook-lv|g" cluster-test.yaml
  elif [ "$1" = "encryption" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}\n    config:\n      encryptedDevice: \"true\"|g" cluster-test.yaml
  elif [ "$1" = "lvm" ]; then
    sed -i "s|#deviceFilter:|devices:\n      - name: \"/dev/test-rook-vg/test-rook-lv\"|g" cluster-test.yaml
  elif [ "$1" = "loop" ]; then
    # add both /dev/sdX1 and loop device to test them at the same time
    sed -i "s|#deviceFilter:|devices:\n      - name: \"${BLOCK}\"\n      - name: \"/dev/loop1\"|g" cluster-test.yaml
  else
    echo "invalid argument: $*" >&2
    exit 1
  fi
  # enable monitoring
  yq w -i -d1 cluster-test.yaml spec.monitoring.enabled true
  kubectl apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.40.0/bundle.yaml
  kubectl create -f monitoring/rbac.yaml

  # create the cluster resources
  kubectl create -f cluster-test.yaml
  kubectl create -f object-test.yaml
  kubectl create -f pool-test.yaml
  kubectl create -f filesystem-test.yaml
  sed -i "/resources:/,/ # priorityClassName:/d" rbdmirror.yaml
  kubectl create -f rbdmirror.yaml
  sed -i "/resources:/,/ # priorityClassName:/d" filesystem-mirror.yaml
  kubectl create -f filesystem-mirror.yaml
  kubectl create -f nfs-test.yaml
  kubectl create -f subvolumegroup.yaml
  deploy_toolbox
}

function deploy_csi_hostnetwork_disabled_cluster() {
  create_cluster_prerequisites
  cd deploy/examples
  sed -i 's/.*CSI_ENABLE_HOST_NETWORK:.*/  CSI_ENABLE_HOST_NETWORK: \"false\"/g' operator.yaml
  deploy_manifest_with_local_build operator.yaml
  if [ $# == 0 ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  elif [ "$1" = "two_osds_in_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}\n    config:\n      osdsPerDevice: \"2\"|g" cluster-test.yaml
  elif [ "$1" = "osd_with_metadata_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}\n    config:\n      metadataDevice: /dev/test-rook-vg/test-rook-lv|g" cluster-test.yaml
  fi
  kubectl create -f nfs-test.yaml
  kubectl create -f cluster-test.yaml
  kubectl create -f filesystem-test.yaml
  deploy_toolbox
}

function wait_for_prepare_pod() {
  get_pod_cmd=(kubectl --namespace rook-ceph get pod --no-headers)
  timeout=450
  start_time="${SECONDS}"
  while [[ $((SECONDS - start_time)) -lt $timeout ]]; do
    pod="$("${get_pod_cmd[@]}" --selector=app=rook-ceph-osd-prepare --output custom-columns=NAME:.metadata.name,PHASE:status.phase | awk 'FNR <= 1')"
    if echo "$pod" | grep 'Running\|Succeeded\|Failed'; then break; fi
    echo 'waiting for at least one osd prepare pod to be running or finished'
    sleep 5
  done
  pod="$("${get_pod_cmd[@]}" --selector app=rook-ceph-osd-prepare --output name | awk 'FNR <= 1')"
  kubectl --namespace rook-ceph logs --follow "$pod"
  timeout=60
  start_time="${SECONDS}"
  while [[ $((SECONDS - start_time)) -lt $timeout ]]; do
    pod="$("${get_pod_cmd[@]}" --selector app=rook-ceph-osd,ceph_daemon_id=0 --output custom-columns=NAME:.metadata.name,PHASE:status.phase)"
    if echo "$pod" | grep 'Running'; then break; fi
    echo 'waiting for OSD 0 pod to be running'
    sleep 1
  done
  # getting the below logs is a best-effort attempt, so use '|| true' to allow failures
  pod="$("${get_pod_cmd[@]}" --selector app=rook-ceph-osd,ceph_daemon_id=0 --output name)" || true
  kubectl --namespace rook-ceph logs "$pod" || true
  job="$(kubectl --namespace rook-ceph get job --selector app=rook-ceph-osd-prepare --output name | awk 'FNR <= 1')" || true
  kubectl -n rook-ceph describe "$job" || true
  kubectl -n rook-ceph describe deployment/rook-ceph-osd-0 || true
}

function wait_for_cleanup_pod() {
  timeout 180 bash <<EOF
until kubectl --namespace rook-ceph logs job/cluster-cleanup-job-$(uname -n); do
  echo "waiting for cleanup up pod to be present"
  sleep 1
done
EOF
  kubectl --namespace rook-ceph logs --follow job/cluster-cleanup-job-"$(uname -n)"
}

function wait_for_ceph_to_be_ready() {
  DAEMONS=$1
  OSD_COUNT=$2
  mkdir -p test
  tests/scripts/validate_cluster.sh "$DAEMONS" "$OSD_COUNT"
  kubectl -n rook-ceph get pods
}

function verify_key_rotation() {
  pvc_name=$(kubectl get pvc -n rook-ceph -l ceph.rook.io/setIndex=0 -o jsonpath='{.items[0].metadata.name}')
  old_key=$(kubectl -n rook-ceph get secrets -l "pvc_name=$pvc_name" -o jsonpath='{.items[0].data.'dmcrypt-key'}' | base64 --decode)
  runtime="3 minutes"
  endtime=$(date -ud "$runtime" +%s)
  while [[ $(date -u +%s) -le $endtime ]]; do
    echo "Time Now: $(date +%H:%M:%S)"
    new_key=$(kubectl -n rook-ceph get secrets -l "pvc_name=$pvc_name" -o jsonpath='{.items[0].data.'dmcrypt-key'}' | base64 --decode)
    if [ "$old_key" != "$new_key" ]; then
      echo "encryption passphrase is successfully rotated"
      exit 0
    fi
    echo "encryption passphrase is not rotated, sleeping for 10 seconds"
    sleep 10s
  done
  new_key=$(kubectl -n rook-ceph get secrets -l "pvc_name=$pvc_name" -o jsonpath='{.items[0].data.'dmcrypt-key'}' | base64 --decode)
  if [ "$old_key" == "$new_key" ]; then
    echo "encryption passphrase is not rotated"
    exit 1
  else
    echo "encryption passphrase is successfully rotated"
  fi
}

function check_ownerreferences() {
  curl -L https://github.com/kubernetes-sigs/kubectl-check-ownerreferences/releases/download/v0.2.0/kubectl-check-ownerreferences-linux-amd64.tar.gz -o kubectl-check-ownerreferences-linux-amd64.tar.gz
  tar xzvf kubectl-check-ownerreferences-linux-amd64.tar.gz
  chmod +x kubectl-check-ownerreferences
  ./kubectl-check-ownerreferences -n rook-ceph
}

function create_LV_on_disk() {
  DEVICE=$1
  VG=test-rook-vg
  LV=test-rook-lv
  sudo sgdisk --zap-all "${DEVICE}"
  sudo vgcreate "$VG" "$DEVICE" || sudo vgcreate "$VG" "$DEVICE" || sudo vgcreate "$VG" "$DEVICE"
  sudo lvcreate -l 100%FREE -n "${LV}" "${VG}"
}

function deploy_first_rook_cluster() {
  BLOCK=$(sudo lsblk | awk '/14G/ || /64G/ {print $1}' | head -1)
  create_cluster_prerequisites
  cd deploy/examples/

  deploy_manifest_with_local_build operator.yaml
  yq w -i -d1 cluster-test.yaml spec.dashboard.enabled false
  yq w -i -d1 cluster-test.yaml spec.storage.useAllDevices false
  yq w -i -d1 cluster-test.yaml spec.storage.deviceFilter "${BLOCK}"1
  kubectl create -f cluster-test.yaml
  deploy_toolbox
}

function deploy_second_rook_cluster() {
  BLOCK=$(sudo lsblk | awk '/14G/ || /64G/ {print $1}' | head -1)
  cd deploy/examples/
  NAMESPACE=rook-ceph-secondary envsubst <common-second-cluster.yaml | kubectl create -f -
  sed -i 's/namespace: rook-ceph/namespace: rook-ceph-secondary/g' cluster-test.yaml
  yq w -i -d1 cluster-test.yaml spec.storage.deviceFilter "${BLOCK}"2
  yq w -i -d1 cluster-test.yaml spec.dataDirHostPath "/var/lib/rook-external"
  kubectl create -f cluster-test.yaml
  yq w -i toolbox.yaml metadata.namespace rook-ceph-secondary
  deploy_toolbox
}

function wait_for_rgw() {
  for _ in {1..120}; do
    if [ "$(kubectl -n "$1" get pod -l app=rook-ceph-rgw --no-headers --field-selector=status.phase=Running | wc -l)" -ge 1 ]; then
      echo "rgw pod is found"
      break
    fi
    echo "waiting for rgw pods"
    sleep 5
  done
  for _ in {1..120}; do
    if [ "$(kubectl -n "$1" get deployment -l app=rook-ceph-rgw -o yaml | yq read - 'items[0].status.readyReplicas')" -ge 1 ]; then
      echo "rgw is ready"
      break
    fi
    echo "waiting for rgw becomes ready"
    sleep 5
  done
}

function verify_operator_log_message() {
  local message="$1"                # param 1: the message to verify exists
  local namespace="${2:-rook-ceph}" # optional param 2: the namespace of the CephCluster (default: rook-ceph)
  kubectl --namespace "$namespace" logs deployment/rook-ceph-operator | grep "$message"
}

function wait_for_operator_log_message() {
  local message="$1"                # param 1: the message to look for
  local timeout="$2"                # param 2: the timeout for waiting for the message to exist
  local namespace="${3:-rook-ceph}" # optional param 3: the namespace of the CephCluster (default: rook-ceph)
  start_time="${SECONDS}"
  while [[ $((SECONDS - start_time)) -lt $timeout ]]; do
    if verify_operator_log_message "$message" "$namespace"; then return 0; fi
    sleep 5
  done
  echo "timed out" >&2 && return 1
}

function restart_operator() {
  local namespace="${1:-rook-ceph}" # optional param 1: the namespace of the CephCluster (default: rook-ceph)
  kubectl --namespace "$namespace" delete pod --selector app=rook-ceph-operator
  # wait for new pod to be running
  get_pod_cmd=(kubectl --namespace "$namespace" get pod --selector app=rook-ceph-operator --no-headers)
  timeout 20 bash -c \
    "until [[ -n \"\$(${get_pod_cmd[*]} --field-selector=status.phase=Running 2>/dev/null)\" ]] ; do echo waiting && sleep 1; done"
  "${get_pod_cmd[@]}"
}

function get_clusterip() {
  local ns=${1?namespace is required}
  local cluster_name=${2?cluster name is required}

  kubectl -n "$ns" get svc "$cluster_name" -o jsonpath="{.spec.clusterIP}"
}

function get_secret_key() {
  local ns=${1?namespace is required}
  local secret_name=${2?secret name is required}
  local key=${3?skey is required}

  kubectl -n "$ns" get secrets "$secret_name" -o jsonpath="{.data.$key}" | base64 --decode
}

function s3cmd() {
  command timeout 20 s3cmd -v --config=s3cfg --access_key="${S3CMD_ACCESS_KEY}" --secret_key="${S3CMD_SECRET_KEY}" "$@"
}

function write_object_read_from_replica_cluster() {
  local write_cluster_ip=${1?ip address of cluster to write to is required}
  local read_cluster_ip=${2?ip address of cluster to read from is required}
  local test_bucket_name=${3?name of the test bucket is required}

  local test_object_name="${test_bucket_name}-1mib-test.dat"
  fallocate -l 1M "$test_object_name"
  # ensure that test file has unique data
  echo "$test_object_name" >>"$test_object_name"

  s3cmd --host="${write_cluster_ip}" mb "s3://${test_bucket_name}"
  s3cmd --host="${write_cluster_ip}" put "$test_object_name" "s3://${test_bucket_name}"

  # Schedule a signal for 60s into the future as a timeout on retrying s3cmd.
  # This voodoo is to avoid running everything under a new shell started by
  # `timeout`, as there would be no way to pass functions to as it wouldn't be
  # a direct sub-shell.
  S3CMD_ERROR=0
  (
    sleep 60
    kill -s SIGUSR1 $$
  ) 2>/dev/null &
  trap "{ S3CMD_ERROR=1; break; }" SIGUSR1

  until s3cmd --host="${read_cluster_ip}" get "s3://${test_bucket_name}/${test_object_name}" "${test_object_name}.get" --force; do
    echo "waiting for object to be replicated"
    sleep 5
  done

  if [[ $S3CMD_ERROR != 0 ]]; then
    echo "s3cmd failed"
    exit $S3CMD_ERROR
  fi

  diff "$test_object_name" "${test_object_name}.get"
}

function test_multisite_object_replication() {
  S3CMD_ACCESS_KEY=$(get_secret_key rook-ceph realm-a-keys access-key)
  readonly S3CMD_ACCESS_KEY
  S3CMD_SECRET_KEY=$(get_secret_key rook-ceph realm-a-keys secret-key)
  readonly S3CMD_SECRET_KEY

  local cluster_1_ip
  cluster_1_ip=$(get_clusterip rook-ceph rook-ceph-rgw-multisite-store)
  local cluster_2_ip
  cluster_2_ip=$(get_clusterip rook-ceph-secondary rook-ceph-rgw-zone-b-multisite-store)

  cd deploy/examples
  cat <<-EOF >s3cfg
	[default]
	host_bucket = no.way
	use_https = False
	EOF

  write_object_read_from_replica_cluster "$cluster_1_ip" "$cluster_2_ip" test1
  write_object_read_from_replica_cluster "$cluster_2_ip" "$cluster_1_ip" test2
}

function create_helm_tag() {
  helm_tag="$(cat _output/version)"
  build_image="$(docker images | awk '/build-/ {print $1}')"
  docker tag "${build_image}" "rook/ceph:${helm_tag}"
}

function deploy_multus() {
  # download the multus daemonset, and remove mem and cpu limits that cause it to crash on minikube
  curl https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset-thick.yml |
    sed -e 's/cpu: /# cpu: /g' -e 's/memory: /# memory: /g' |
    kubectl apply -f -

  # install whereabouts
  kubectl apply \
    -f https://raw.githubusercontent.com/k8snetworkplumbingwg/whereabouts/master/doc/crds/daemonset-install.yaml \
    -f https://github.com/k8snetworkplumbingwg/whereabouts/raw/master/doc/crds/whereabouts.cni.cncf.io_ippools.yaml \
    -f https://github.com/k8snetworkplumbingwg/whereabouts/raw/master/doc/crds/whereabouts.cni.cncf.io_overlappingrangeipreservations.yaml

  # create the rook-ceph namespace if it doesn't exist, the NAD will go in this namespace
  kubectl create namespace rook-ceph || true

  # install network attachment definitions
  IFACE="eth0" # the runner has eth0 so we don't need any heuristics to find the interface
  kubectl apply -f - <<EOF
---
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: public-net
  namespace: rook-ceph
  labels:
  annotations:
spec:
  config: '{ "cniVersion": "0.3.0", "type": "macvlan", "master": "$IFACE", "mode": "bridge", "ipam": { "type": "whereabouts", "range": "192.168.20.0/24" } }'
---
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: cluster-net
  namespace: rook-ceph
  labels:
  annotations:
spec:
  config: '{ "cniVersion": "0.3.0", "type": "macvlan", "master": "$IFACE", "mode": "bridge", "ipam": { "type": "whereabouts", "range": "192.168.21.0/24" } }'
EOF
}

function deploy_multus_cluster() {
  cd deploy/examples
  sed -i 's/.*ROOK_CSI_ENABLE_NFS:.*/  ROOK_CSI_ENABLE_NFS: \"true\"/g' operator.yaml
  deploy_manifest_with_local_build operator.yaml
  deploy_toolbox
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-multus-test.yaml
  kubectl create -f cluster-multus-test.yaml
  kubectl create -f filesystem-test.yaml
  kubectl create -f nfs-test.yaml
}

function test_multus_connections() {
  EXEC='kubectl -n rook-ceph exec -t deploy/rook-ceph-tools -- ceph --connect-timeout 10'
  # each OSD should exist on both public and cluster network
  $EXEC osd dump | grep osd.0 | grep "192.168.20." | grep "192.168.21."
  # MDSes should exist on public network and NOT on cluster network
  $EXEC fs dump | grep myfs-a | grep "192.168.20." | grep -v "192.168.21."
}

function create_operator_toolbox() {
  cd deploy/examples
  sed -i "s|image: rook/ceph:.*|image: rook/ceph:local-build|g" toolbox-operator-image.yaml
  kubectl create -f toolbox-operator-image.yaml
}

function wait_for_ceph_csi_configmap_to_be_updated {
  timeout 60 bash <<EOF
until [[ $(kubectl -n rook-ceph get configmap rook-ceph-csi-config -o jsonpath="{.data.csi-cluster-config-json}" | jq .[0].rbd.netNamespaceFilePath) != "null" ]]; do
  echo "waiting for ceph csi configmap to be updated with rbd.netNamespaceFilePath"
  sleep 5
done
EOF
  timeout 60 bash <<EOF
until [[ $(kubectl -n rook-ceph get configmap rook-ceph-csi-config -o jsonpath="{.data.csi-cluster-config-json}" | jq .[0].cephFS.netNamespaceFilePath) != "null" ]]; do
  echo "waiting for ceph csi configmap to be updated with cephFS.netNamespaceFilePath"
  sleep 5
done
EOF
  timeout 60 bash <<EOF
until [[ $(kubectl -n rook-ceph get configmap rook-ceph-csi-config -o jsonpath="{.data.csi-cluster-config-json}" | jq .[0].nfs.netNamespaceFilePath) != "null" ]]; do
  echo "waiting for ceph csi configmap to be updated with nfs.netNamespaceFilePath"
  sleep 5
done
EOF
}

function test_csi_rbd_workload {
  cd deploy/examples/csi/rbd
  sed -i 's|size: 3|size: 1|g' storageclass.yaml
  sed -i 's|requireSafeReplicaSize: true|requireSafeReplicaSize: false|g' storageclass.yaml
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/csi-rbdplugin -c csi-rbdplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=csi-rbdplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csirbd-demo-pod -- ls -alh /var/lib/www/html/
}

function test_csi_cephfs_workload {
  cd deploy/examples/csi/cephfs
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/csi-cephfsplugin -c csi-cephfsplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=csi-cephfsplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csicephfs-demo-pod -- ls -alh /var/lib/www/html/
}

function test_csi_nfs_workload {
  cd deploy/examples/csi/nfs
  sed -i "s|#- debug|- nolock|" storageclass.yaml
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=csi-nfsplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csinfs-demo-pod -- ls -alh /var/lib/www/html/
}

function install_minikube_with_none_driver() {
  CRICTL_VERSION="v1.28.0"
  MINIKUBE_VERSION="v1.31.2"

  sudo apt update
  sudo apt install -y conntrack socat
  curl -LO https://storage.googleapis.com/minikube/releases/$MINIKUBE_VERSION/minikube_latest_amd64.deb
  sudo dpkg -i minikube_latest_amd64.deb
  rm -f minikube_latest_amd64.deb

  curl -LO https://github.com/Mirantis/cri-dockerd/releases/download/v0.3.4/cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb
  sudo dpkg -i cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb
  rm -f cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb

  wget https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo tar zxvf crictl-$CRICTL_VERSION-linux-amd64.tar.gz -C /usr/local/bin
  rm -f crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo sysctl fs.protected_regular=0

  CNI_PLUGIN_VERSION="v1.3.0"
  CNI_PLUGIN_TAR="cni-plugins-linux-amd64-$CNI_PLUGIN_VERSION.tgz" # change arch if not on amd64
  CNI_PLUGIN_INSTALL_DIR="/opt/cni/bin"

  curl -LO "https://github.com/containernetworking/plugins/releases/download/$CNI_PLUGIN_VERSION/$CNI_PLUGIN_TAR"
  sudo mkdir -p "$CNI_PLUGIN_INSTALL_DIR"
  sudo tar -xf "$CNI_PLUGIN_TAR" -C "$CNI_PLUGIN_INSTALL_DIR"
  rm "$CNI_PLUGIN_TAR"

  export MINIKUBE_HOME=$HOME CHANGE_MINIKUBE_NONE_USER=true KUBECONFIG=$HOME/.kube/config
  sudo -E minikube start --kubernetes-version="$1" --driver=none --memory 6g --cpus=2 --addons ingress --cni=calico
}

FUNCTION="$1"
shift # remove function arg now that we've recorded it
# call the function with the remainder of the user-provided args
# -e, -E, and -o=pipefail will ensure this script returns a failure if a part of the function fails
$FUNCTION "$@"
