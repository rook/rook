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

REPO_DIR="$(readlink -f -- "${BASH_SOURCE%/*}/../..")"
NETWORK_ERROR="connection reset by peer"
SERVICE_UNAVAILABLE_ERROR="Service Unavailable"
INTERNAL_ERROR="INTERNAL_ERROR"
INTERNAL_SERVER_ERROR="500 Internal Server Error"

# Architecture detection
ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
  ARCH_SUFFIX="arm64"
else
  ARCH_SUFFIX="amd64"
fi

#############
# FUNCTIONS #
#############

# Create one or more real block devices via a local iSCSI (LIO) target so that
# tests have access to genuine SCSI disks (/dev/sd*). The argument is the number
# of disks to create (default 1). All disks share a single target/TPG and are
# exposed as separate LUNs, so a single initiator login attaches them all.
function create_extra_disk() {
  local count="${1:-1}"
  sudo apt install -y targetcli-fb open-iscsi
  local init_iqn=iqn.2026-02.initiator.local
  local target_iqn=iqn.2026-02.target.local:disk1
  echo "InitiatorName=${init_iqn}" | sudo tee /etc/iscsi/initiatorname.iscsi >/dev/null
  # The target and its initiator ACL are shared by all disks; create them once.
  # "|| true" keeps re-invocation (to append more disks) idempotent.
  sudo targetcli /iscsi create ${target_iqn} 2>/dev/null || true
  sudo targetcli /iscsi/${target_iqn}/tpg1/acls create ${init_iqn} 2>/dev/null || true
  # Count disks that already exist so additional disks append rather than
  # clobber lun0 (LIO auto-assigns LUN numbers sequentially in creation order).
  local existing=0 f
  for f in ~/iscsi-disk*.img; do
    if [ -e "$f" ]; then
      existing=$((existing + 1))
    fi
  done
  local j i lun
  for j in $(seq 1 "$count"); do
    i=$((existing + j))
    lun=$((i - 1))
    truncate -s 75G ~/iscsi-disk${i}.img
    sudo targetcli /backstores/fileio create disk${i} ~/iscsi-disk${i}.img 75G
    # "luns create" auto-maps the new LUN into the existing ACL as
    # lun${lun} -> mapped_lun${lun}, so the explicit mapping below is a fallback
    # for targetcli versions that do not auto-map; "|| true" tolerates the
    # "already exists" error once it has been auto-mapped.
    sudo targetcli /iscsi/${target_iqn}/tpg1/luns create /backstores/fileio/disk${i}
    sudo targetcli /iscsi/${target_iqn}/tpg1/acls/${init_iqn} create tpg_lun_or_backstore=lun${lun} mapped_lun=${lun} 2>/dev/null || true
  done
  sudo iscsiadm -m discovery -t sendtargets -p 127.0.0.1
  sudo iscsiadm -m node --login 2>/dev/null || true
  sudo iscsiadm -m node -R 2>/dev/null || true # rescan to pick up newly-added LUNs
  sudo udevadm settle 2>/dev/null || true      # wait for the new /dev/sd* to appear
}

# Print the basename of the first non-boot/loop/nbd whole disk, provisioning an
# iSCSI disk if none exist. Thin wrapper over find_extra_block_devs(); kept as a
# convenience for its many callers that expect a single basename.
function find_extra_block_dev() {
  find_extra_block_devs 1 | head -1
}

# find_extra_block_devs [min_count]
# Print ALL non-boot/loop/nbd whole-disk basenames, one per line, in stable
# lsblk order, provisioning additional iSCSI disks if fewer than min_count exist.
# This is the single source of truth for disk discovery; find_extra_block_dev()
# returns the first and find_second_block_dev() the second. Debug goes to stderr
# so stdout stays clean.
function find_extra_block_devs() {
  local min_count="${1:-1}"
  # shellcheck disable=SC2005 # redirect doesn't work with sudo, so use echo
  echo "$(sudo lsblk)" >/dev/stderr # print lsblk output to stderr for debugging
  # relevant lsblk --pairs example: (MOUNTPOINT identifies boot partition)(PKNAME is Parent dev ID)
  # NAME="sda15" SIZE="106M" TYPE="part" MOUNTPOINT="/boot/efi" PKNAME="sda"
  # NAME="sdb"   SIZE="75G"  TYPE="disk" MOUNTPOINT=""          PKNAME=""
  # NAME="sdb1"  SIZE="75G"  TYPE="part" MOUNTPOINT="/mnt"      PKNAME="sdb"
  boot_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | grep boot | awk '{print $2}')"
  local devs count
  # --nodeps ignores partitions
  devs="$(sudo lsblk --noheading --list --nodeps --output KNAME | egrep -v "($boot_dev|loop|nbd)")"
  count="$(echo "$devs" | grep -c . || true)"
  if [ "$count" -lt "$min_count" ]; then
    create_extra_disk "$((min_count - count))" >/dev/stderr
    devs="$(sudo lsblk --noheading --list --nodeps --output KNAME | egrep -v "($boot_dev|loop|nbd)")"
  fi
  echo "  == find_extra_block_devs(): wanted>=${min_count} devs='$(echo "$devs" | tr '\n' ' ')'" >/dev/stderr
  echo "$devs" # output of function
}

# Print the basename of a second whole disk, distinct from the primary disk
# returned by find_extra_block_dev(). Ensures at least two disks exist first
# (provisioning an iSCSI disk if needed). Debug goes to stderr; stdout is clean.
function find_second_block_dev() {
  local devs
  mapfile -t devs < <(find_extra_block_devs 2)
  echo "${devs[1]}" # the second disk; find_extra_block_dev() returns the first
}

function block_dev() {
  declare -g DEFAULT_BLOCK_DEV
  : "${DEFAULT_BLOCK_DEV:=/dev/$(block_dev_basename)}"

  echo "$DEFAULT_BLOCK_DEV"
}

function block_dev_basename() {
  declare -g DEFAULT_BLOCK_DEV_BASENAME
  : "${DEFAULT_BLOCK_DEV_BASENAME:=$(find_extra_block_dev)}"

  echo "$DEFAULT_BLOCK_DEV_BASENAME"
}

function install_deps() {
  sudo wget https://github.com/mikefarah/yq/releases/download/3.4.1/yq_linux_${ARCH_SUFFIX} -O /usr/local/bin/yq
  sudo chmod +x /usr/local/bin/yq
}

function install_nvme_initiator_prerequisites() {
  sudo apt-get update
  sudo apt-get install -y nvme-cli
  sudo apt-get install -y "linux-modules-extra-$(uname -r)" || sudo apt-get install -y linux-modules-extra-azure || true
  sudo modprobe nvme_fabrics || true
  sudo modprobe nvme_tcp || true
  if [[ ! -d /sys/module/nvme_tcp ]]; then
    echo "nvme_tcp kernel module is unavailable on this runner kernel: $(uname -r)"
    exit 1
  fi
}

function print_k8s_cluster_status() {
  kubectl cluster-info
  kubectl get pods -n kube-system
}

function use_local_disk() {
  BLOCK_DATA_PART="$(block_dev)1"
  sudo apt purge snapd -y
  sudo dmsetup version || true
  sudo swapoff --all --verbose
  if mountpoint -q /mnt; then
    sudo umount /mnt
    # search for the device since it keeps changing between sda and sdb
    sudo wipefs --all --force "$BLOCK_DATA_PART"
  else
    # it's the hosted runner!
    sudo sgdisk --zap-all -- "$(block_dev)"
    sudo dd if=/dev/zero of="$(block_dev)" bs=1M count=10 oflag=direct,dsync
    sudo parted -s "$(block_dev)" mklabel gpt
  fi
  sudo lsblk
}

function use_local_disk_for_integration_test() {
  sudo apt purge snapd -y
  sudo udevadm control --log-priority=debug
  sudo swapoff --all --verbose

  # Create an extra disk if doesn't exist.
  : "$(block_dev)"
  sudo lsblk

  # Stop udev from watching the OSD disk, on every kind of runner. Without this, every
  # close-after-write on the device retriggers a udev probe of the device, and the resulting
  # re-probe storm during OSD restarts has been seen to wedge ceph-volume activate in
  # uninterruptible sleep on the block device lock on runners using the iSCSI fallback disk.
  # See https://github.com/rook/rook/issues/8975 and https://access.redhat.com/solutions/1465913
  echo "ACTION==\"add|change\", KERNEL==\"$(block_dev_basename)\", OPTIONS:=\"nowatch\"" | sudo tee -a /etc/udev/rules.d/99-z-rook-nowatch.rules
  sudo udevadm control --reload-rules || true
  sudo udevadm trigger || true
  time sudo udevadm settle || true

  unset pipefail
  mountpoint -q /mnt || return 0
  set pipefail
  sudo umount /mnt
  sudo sed -i.bak '/\/mnt/d' /etc/fstab
  # search for the device since it keeps changing between sda and sdb
  PARTITION="$(block_dev)1"
  sudo wipefs --all --force "$PARTITION"
  sudo dd if=/dev/zero of="${PARTITION}" bs=1M count=1
  sudo lsblk --bytes
  # add a udev rule to force the disk partitions to ceph
  # we have observed that some runners keep detaching/re-attaching the additional disk overriding the permissions to the default root:disk
  # for more details see: https://github.com/rook/rook/issues/7405
  echo "SUBSYSTEM==\"block\", ATTR{size}==\"29356032\", ACTION==\"add\", RUN+=\"/bin/chown 167:167 $PARTITION\"" | sudo tee -a /etc/udev/rules.d/01-rook.rules
  # The partition is still getting reloaded occasionally during operation. See https://github.com/rook/rook/issues/8975
  # Try issuing some disk-inspection commands to jog the system so it won't reload the partitions
  # during OSD provisioning.
  sudo udevadm control --reload-rules || true
  sudo udevadm trigger || true
  time sudo udevadm settle || true
  sudo partprobe || true
  sudo lsblk --noheadings --pairs "$(block_dev)" || true
  sudo sgdisk --print "$(block_dev)" || true
  sudo udevadm info --query=property "$(block_dev)" || true
  sudo lsblk --noheadings --pairs "${PARTITION}" || true
  journalctl -o short-precise --dmesg | tail -40 || true
  cat /etc/fstab || true
}

function create_partitions_for_osds() {
  tests/scripts/create-bluestore-partitions.sh --disk "$(block_dev)" --osd-count 2
  sudo lsblk
}

function create_bluestore_partitions_and_pvcs() {
  BLOCK_PART="$(block_dev)2"
  DB_PART="$(block_dev)1"
  tests/scripts/create-bluestore-partitions.sh --disk "$(block_dev)" --bluestore-type block.db --osd-count 1
  tests/scripts/localPathPV.sh "$BLOCK_PART" "$DB_PART"
}

function create_bluestore_partitions_and_pvcs_for_wal() {
  BLOCK_PART="$(block_dev)3"
  DB_PART="$(block_dev)1"
  WAL_PART="$(block_dev)2"
  tests/scripts/create-bluestore-partitions.sh --disk "$(block_dev)" --bluestore-type block.wal --osd-count 1
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
    if ! o=$(make -j"$(nproc)" "$build_type"); then
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
    docker tag "$(docker images | awk '/build-/ {print $1}')" docker.io/rook/ceph:local-build
  fi
}

function build_rook_all() {
  build_rook build.all
}

function validate_yaml() {
  cd "${REPO_DIR}/deploy/examples"
  kubectl create -f crds.yaml -f common.yaml -f csi-operator.yaml

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
  manifests="$(find . -maxdepth 1 -type f -name '*.yaml' -and -not -name '*openshift*' -and -not -name 'scc*' -and -not -name 'kustomization*')"
  with_f_arg="$(echo "$manifests" | awk '{printf " -f %s",$1}')" # don't add newline
  # shellcheck disable=SC2086 # '-f manifest1.yaml -f manifest2.yaml etc.' should not be quoted
  kubectl create ${with_f_arg} --dry-run=client
}

function create_cluster_prerequisites() {
  # this might be called from another function that has already done a cd
  (cd "${REPO_DIR}/deploy/examples" && kubectl create -f crds.yaml -f common.yaml -f csi-operator.yaml)
}

function remove_cluster_prerequisites() {
  (cd "${REPO_DIR}/deploy/examples" && kubectl delete -f crds.yaml -f common.yaml)
}

function deploy_manifest_with_local_build() {
  if [[ "$1" == "deploy/examples/operator.yaml" || "$1" == "operator.yaml" ]]; then
    sed -i "s|deployCsiAddons: false|deployCsiAddons: true|g" $1
    sed -i "s|replicas: 2|replicas: 1|g" $1
  fi
  if [[ "$USE_LOCAL_BUILD" != "false" ]]; then
    sed -i "s|image: docker.io/rook/ceph:.*|image: docker.io/rook/ceph:local-build|g" $1
  fi
  sed -i "s|ROOK_LOG_LEVEL:.*|ROOK_LOG_LEVEL: DEBUG|g" "$1"
  kubectl create -f $1
}

# Deploy toolbox with same ceph version as the cluster-test for ci
function deploy_toolbox() {
  cd "${REPO_DIR}/deploy/examples"
  sed -i 's/image: quay\.io\/ceph\/ceph:.*/image: quay.io\/ceph\/ceph:v18/' toolbox.yaml
  local ns
  ns=$(yq r toolbox.yaml metadata.namespace 2>/dev/null)
  timeout 300 bash -c 'until kubectl get secret rook-ceph-mon -n "$1" &>/dev/null && kubectl get cm rook-ceph-mon-endpoints -n "$1" &>/dev/null; do sleep 2; done' _ "${ns}"
  kubectl create -f toolbox.yaml
  kubectl -n "${ns}" wait --for=condition=available deployment/rook-ceph-tools --timeout=300s
}

function replace_ceph_image() {
  local file="$1"                                # parameter 1: the file in which to replace the ceph image
  local ceph_image="${2?ceph_image is required}" # parameter 2: the new ceph image to use

  # check for ceph_image being an empty string
  if [ -z "$ceph_image" ]; then
    echo "ceph_image may not be an empty string"
    exit 1
  fi

  sed -i "s|image: .*ceph/ceph:.*|image: ${ceph_image}|g" "${file}"
}

# Deploy the operator, a CephCluster, and the toolbox. This is intended to be a
# minimal deployment generic enough to be used by most canary tests. Each
# canary test should be installing its own set of resources as a job step or
# using dedicated helper functions.
function deploy_cluster() {
  cd "${REPO_DIR}/deploy/examples"

  kubectl create -f networkpolicy.yaml
  deploy_manifest_with_local_build operator.yaml

  if [ $# == 0 ]; then
    sed -i "s|#deviceFilter:|deviceFilter: $(block_dev_basename)|g" cluster-test.yaml
  elif [ "$1" = "two_osds_in_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: $(block_dev_basename)\n    config:\n      osdsPerDevice: \"2\"|g" cluster-test.yaml
  elif [ "$1" = "osd_with_metadata_device" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: $(block_dev_basename)\n    config:\n      metadataDevice: /dev/test-rook-vg/test-rook-lv|g" cluster-test.yaml
  elif [ "$1" = "osd_with_metadata_partition_device" ]; then
    yq w -i -d0 cluster-test.yaml spec.storage.devices[0].name "$(block_dev_basename)2"
    yq w -i -d0 cluster-test.yaml spec.storage.devices[0].config.metadataDevice "$(block_dev_basename)1"
  elif [ "$1" = "encryption" ]; then
    sed -i "s|#deviceFilter:|deviceFilter: $(block_dev_basename)\n    config:\n      encryptedDevice: \"true\"|g" cluster-test.yaml
  elif [ "$1" = "lvm" ]; then
    sed -i "s|#deviceFilter:|devices:\n      - name: \"/dev/test-rook-vg/test-rook-lv\"|g" cluster-test.yaml
  elif [ "$1" = "two_raw_disks" ]; then
    # use two real disks as raw OSDs; ROOK_DATA_DEV and ROOK_EXTRA_DEV are
    # basenames (e.g. sdb, sdc) exported by the job. An explicit devices list
    # plus useAllDevices=false pins the OSD count regardless of any stray disk.
    sed -i "s|#deviceFilter:|devices:\n      - name: \"${ROOK_DATA_DEV}\"\n      - name: \"${ROOK_EXTRA_DEV}\"|g" cluster-test.yaml
    yq w -i -d0 cluster-test.yaml spec.storage.useAllDevices false
  else
    echo "invalid argument: $*" >&2
    exit 1
  fi

  # enable monitoring
  yq w -i -d0 cluster-test.yaml spec.monitoring.enabled true
  kubectl create -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.82.0/bundle.yaml
  kubectl create -f monitoring/rbac.yaml

  kubectl create -f cluster-test.yaml

  deploy_toolbox
}

# These resources were extracted from the original deploy_cluster(), which was
# deploying a smorgasbord of resources used by multiple canary tests.
#
# Use of this function is discouraged. Existing users should migrate away and
# deploy only the necessary resources for the test scenario in their own job
# steps.
#
# The addition of new resources this function is forbidden!
function deploy_all_additional_resources_on_cluster() {
  cd "${REPO_DIR}/deploy/examples"

  kubectl create -f object-shared-pools-test.yaml
  kubectl create -f object-a.yaml
  kubectl create -f object-b.yaml
  kubectl create -f pool-test.yaml
  kubectl create -f filesystem-test.yaml
  sed -i "/resources:/,/ # priorityClassName:/d" rbdmirror.yaml
  kubectl create -f rbdmirror.yaml
  sed -i "/resources:/,/ # priorityClassName:/d" filesystem-mirror.yaml
  kubectl create -f filesystem-mirror.yaml
  kubectl create -f nfs-test.yaml
  kubectl create -f subvolumegroup.yaml
}

function wait_for_prepare_pod() {
  # wait for a mon to be created
  # most of this time is likely waiting for the detect version job to pull the ceph image
  get_pod_cmd=(kubectl --namespace rook-ceph get pod --no-headers)
  timeout=600
  start_time="${SECONDS}"
  while [[ $((SECONDS - start_time)) -lt $timeout ]]; do
    pod="$("${get_pod_cmd[@]}" --selector=app=rook-ceph-mon --output custom-columns=NAME:.metadata.name,PHASE:status.phase)"
    if echo "$pod" | grep 'rook-ceph-mon-a'; then break; fi
    echo 'waiting for mon.a to be created'
    sleep 5
  done

  # wait for at an osd prepare pod to complete
  OSD_COUNT=$1
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

  # wait for an osd daemon pod to start
  timeout=60
  start_time="${SECONDS}"
  while [[ $((SECONDS - start_time)) -lt $timeout ]]; do
    pod_count="$("${get_pod_cmd[@]}" --selector app=rook-ceph-osd --output custom-columns=NAME:.metadata.name,PHASE:status.phase | grep --count 'Running' || true)"
    if [ "$pod_count" -ge "$OSD_COUNT" ]; then break; fi
    echo 'waiting for $OSD_COUNT OSD pod(s) to be running'
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
  DEVICE_NAME="$(tests/scripts/github-action-helper.sh find_extra_block_dev)"
  create_cluster_prerequisites
  cd "${REPO_DIR}/deploy/examples"

  deploy_manifest_with_local_build operator.yaml
  yq w -i -d0 cluster-test.yaml spec.dashboard.enabled false
  yq w -i -d0 cluster-test.yaml spec.storage.useAllDevices false
  yq w -i -d0 cluster-test.yaml spec.storage.deviceFilter "${DEVICE_NAME}"[1-3]

  kubectl create -f cluster-test.yaml
  deploy_toolbox
}

# deploy_second_rook_cluster will only work if there are 6 disk or partitions available
# as we are picking "${DEVICE_NAME}"[4-6]
function deploy_second_rook_cluster() {
  DEVICE_NAME="$(tests/scripts/github-action-helper.sh find_extra_block_dev)"
  cd "${REPO_DIR}/deploy/examples"
  NAMESPACE=rook-ceph-secondary envsubst <common-second-cluster.yaml | kubectl create -f -
  sed -i 's/namespace: rook-ceph/namespace: rook-ceph-secondary/g' cluster-test.yaml
  yq w -i -d0 cluster-test.yaml spec.storage.deviceFilter "${DEVICE_NAME}"[4-6]

  yq w -i -d0 cluster-test.yaml spec.dataDirHostPath "/var/lib/rook-external"
  kubectl create -f cluster-test.yaml
  yq w -i toolbox.yaml metadata.namespace rook-ceph-secondary
  deploy_toolbox
}

function wait_for() {
  local kind=${1?kind is required}
  local name=${2?resource name is required}
  local ns=${3:-rook-ceph}
  local timeout=${4:-120}
  local status=${5:-Ready}

  local start_time="${SECONDS}"
  local elapsed_time=0
  while [[ $elapsed_time -lt $timeout ]]; do
    if [[ "$(kubectl -n "$ns" get "$kind" "$name" -o 'jsonpath={..status.phase}')" == "$status" ]]; then
      echo "${kind}/${name} in ${ns} is ${status} -  elapsed time ${elapsed_time}s"
      return 0
    fi

    elapsed_time=$((SECONDS - start_time))
    echo "waiting for ${kind}/${name} in ${ns} to be ${status} - elapsed time ${elapsed_time}s"
    sleep 5
  done

  echo "timed out waiting for ${kind}/${name} in ${ns} to be ${status} - elapsed time ${elapsed_time}s " >&2
  exit 1
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
  command timeout 200 s3cmd -v --config=s3cfg --access_key="${S3CMD_ACCESS_KEY}" --secret_key="${S3CMD_SECRET_KEY}" "$@"
}

function retry() {
  local retries=$1; shift
  local attempt
  for (( attempt=1; attempt<=retries; attempt++ )); do
    if "$@"; then return 0; fi
    echo "$* failed (attempt ${attempt}/${retries}), retrying in 5s..."
    sleep 5
  done
  echo "$* failed after ${retries} attempts"
  exit 1
}

function retry_for() {
  local timeout_seconds=${1?timeout in seconds is required}
  shift
  local deadline=$((SECONDS + timeout_seconds))
  while true; do
    if "$@"; then return 0; fi
    if [[ $SECONDS -ge $deadline ]]; then
      echo "$* failed after retrying for ${timeout_seconds}s"
      return 1
    fi
    echo "$* failed, retrying in 5s..."
    sleep 5
  done
}

function wait_for_rgw_http() {
  local endpoint_ip=${1?rgw endpoint ip is required}
  local timeout_seconds=${2:-300}

  # Require a few consecutive successes: RGWs pause their HTTP frontends while reloading the
  # realm after RGW configuration period changes, so a single success can land between pauses.
  local want_consecutive=3
  local consecutive=0
  local deadline=$((SECONDS + timeout_seconds))
  local code
  while [[ $SECONDS -lt $deadline ]]; do
    code=$(curl --silent --output /dev/null --max-time 5 --write-out '%{http_code}' "http://${endpoint_ip}:80/") || code="000"
    if [[ "$code" == "200" ]]; then
      consecutive=$((consecutive + 1))
      if [[ $consecutive -ge $want_consecutive ]]; then
        echo "RGW at ${endpoint_ip} answered ${consecutive} consecutive requests"
        return 0
      fi
    else
      echo "RGW at ${endpoint_ip} is not serving yet (HTTP code ${code})"
      consecutive=0
    fi
    sleep 2
  done
  echo "timed out after ${timeout_seconds}s waiting for RGW at ${endpoint_ip} to serve HTTP"
  return 1
}

function wait_for_sync_status() {
  local ns=${1?namespace is required}
  local zone=${2?zone name is required}
  local want=${3?expected sync status line is required}
  local timeout_seconds=${4:-600}

  local deadline=$((SECONDS + timeout_seconds))
  local status=""
  while [[ $SECONDS -lt $deadline ]]; do
    status=$(kubectl -n "$ns" exec deploy/rook-ceph-tools -- \
      radosgw-admin sync status --rgw-realm=realm-a --rgw-zonegroup=zonegroup-a --rgw-zone="$zone") || status=""
    # require a non-empty status so a failed exec can't match a stale/empty buffer
    if [[ -n "$status" ]] && grep -q "$want" <<<"$status"; then
      echo "zone ${zone} reports \"${want}\""
      return 0
    fi
    echo "waiting for zone ${zone} to report \"${want}\""
    sleep 10
  done
  echo "timed out after ${timeout_seconds}s waiting for zone ${zone} to report \"${want}\"; last sync status:"
  echo "$status"
  return 1
}

# nudge_multisite_secondary forces the secondary zone to converge on the latest period when it
# has wedged. The master pushes each new period to the secondary's RGW, but that push is
# rejected (HTTP 403) until the realm system user is resolvable locally on the secondary; when
# it starts out unresolvable the master keeps re-pushing identically and the secondary never
# recovers, so its metadata sync never leaves the "failed to read sync status" state. Pulling
# the period directly (outbound auth, which works) and restarting the RGW so it re-runs metadata
# sync init against the now-ready master breaks the wedge. This mirrors ceph's own multisite QA,
# which restarts zone gateways after the period is committed.
function nudge_multisite_secondary() {
  echo "nudging the secondary zone to converge: pulling the latest period and restarting its RGW"
  kubectl -n rook-ceph-secondary exec deploy/rook-ceph-tools -- \
    radosgw-admin period pull --rgw-realm=realm-a || true
  kubectl -n rook-ceph-secondary rollout restart deploy/rook-ceph-rgw-zone-b-multisite-store-a
  kubectl -n rook-ceph-secondary rollout status deploy/rook-ceph-rgw-zone-b-multisite-store-a --timeout=120s
}

function wait_for_multisite_sync_established() {
  # Wait until both zones report multisite sync fully established before exercising
  # replication, mirroring the checkpoints ceph's own multisite QA performs before asserting
  # anything. This also rides out the RGW frontend pauses caused by the configuration period
  # changes that follow the second zone joining the zonegroup.
  #
  # The secondary's metadata sync can wedge on a fresh setup (see nudge_multisite_secondary); if
  # it has not initialized within a couple of minutes, nudge it to converge rather than waiting
  # out a doomed timeout, then retry. Healthy setups pass the first attempt in seconds, so the
  # nudge only ever runs when the sync is genuinely stuck.
  local attempt
  for attempt in 1 2 3; do
    if wait_for_sync_status rook-ceph-secondary zone-b "metadata is caught up with master" 120; then
      break
    fi
    if [[ $attempt -ge 3 ]]; then
      echo "zone-b metadata sync never established after ${attempt} attempts"
      return 1
    fi
    nudge_multisite_secondary
  done
  # Data sync legitimately reports caught up at 0/0 shards before any objects are written, so the
  # meaningful precondition is that metadata sync (above) has actually initialized.
  wait_for_sync_status rook-ceph-secondary zone-b "data is caught up with source" 300
  wait_for_sync_status rook-ceph zone-a "data is caught up with source" 300
}

function dump_multisite_diagnostics() {
  set +e
  local ns zone
  for ns in rook-ceph rook-ceph-secondary; do
    # sync status without an explicit zone falls back to the unrelated "default" zone, so name
    # the realm/zonegroup/zone for each cluster.
    if [[ "$ns" == "rook-ceph" ]]; then zone="zone-a"; else zone="zone-b"; fi
    echo "===== ${ns}: pods"
    kubectl -n "$ns" get pods -o wide
    echo "===== ${ns}: rgw pod details"
    kubectl -n "$ns" describe pods -l app=rook-ceph-rgw
    echo "===== ${ns}: rgw pod logs"
    kubectl -n "$ns" logs -l app=rook-ceph-rgw --all-containers --tail=100
    echo "===== ${ns}: sync status (${zone})"
    kubectl -n "$ns" exec deploy/rook-ceph-tools -- \
      radosgw-admin sync status --rgw-realm=realm-a --rgw-zonegroup=zonegroup-a --rgw-zone="$zone"
    echo "===== ${ns}: period"
    kubectl -n "$ns" exec deploy/rook-ceph-tools -- radosgw-admin period get --rgw-realm=realm-a
  done
  set -e
  return 0
}

function write_object_read_from_replica_cluster() {
  local write_cluster_ip=${1?ip address of cluster to write to is required}
  local read_cluster_ip=${2?ip address of cluster to read from is required}
  local test_bucket_name=${3?name of the test bucket is required}
  local write_zone=${4?name of the zone being written to is required}
  local read_zone=${5?name of the zone being read from is required}
  local read_cluster_ns=${6?namespace of the cluster being read from is required}

  local test_object_name="${test_bucket_name}-1mib-test.dat"
  fallocate -l 1M "$test_object_name"

  # ensure that test file has unique data
  echo "$test_object_name" >>"$test_object_name"

  # RGWs pause their HTTP frontends to reload the realm whenever the RGW configuration period
  # changes, and period changes keep happening for a while after the second zone joins the
  # zonegroup. While a frontend is paused, connections are refused, so give the writes a time
  # budget rather than a fixed number of quick retries.
  retry_for 120 s3cmd --host="${write_cluster_ip}" mb "s3://${test_bucket_name}"
  retry_for 120 s3cmd --host="${write_cluster_ip}" put "$test_object_name" "s3://${test_bucket_name}"

  # Wait until the reading zone's sync position for the bucket catches up with the source
  # zone's position, the same marker-based barrier ceph's own multisite QA uses. The bucket
  # metadata has to arrive on the reading zone via metadata sync before the checkpoint can
  # resolve the bucket at all, hence the outer retry.
  retry_for 120 kubectl -n "$read_cluster_ns" exec deploy/rook-ceph-tools -- \
    radosgw-admin bucket sync checkpoint --rgw-realm=realm-a --rgw-zonegroup=zonegroup-a --rgw-zone="$read_zone" \
    --bucket="$test_bucket_name" --source-zone="$write_zone" --retry-delay-ms=5000 --timeout-sec=300

  # The checkpoint confirms the bucket's data sync markers are caught up at the RADOS level, but
  # the reading RGW can still briefly serve 404 for the freshly synced object until it refreshes,
  # so give the read a generous budget rather than asserting it is immediately available.
  retry_for 180 s3cmd --host="${read_cluster_ip}" get "s3://${test_bucket_name}/${test_object_name}" "${test_object_name}.get" --force

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

  cd "${REPO_DIR}/deploy/examples"
  cat <<-EOF >s3cfg
	[default]
	host_bucket = no.way
	use_https = False
	EOF

  # both RGWs must be reliably serving before anything is written
  wait_for_rgw_http "$cluster_1_ip"
  wait_for_rgw_http "$cluster_2_ip"

  write_object_read_from_replica_cluster "$cluster_1_ip" "$cluster_2_ip" test1 zone-a zone-b rook-ceph-secondary
  write_object_read_from_replica_cluster "$cluster_2_ip" "$cluster_1_ip" test2 zone-b zone-a rook-ceph
}

function create_helm_tag() {
  helm_tag="$(cat _output/version)"
  build_image="$(docker images | awk '/build-/ {print $1}')"
  docker tag "${build_image}" "rook/ceph:${helm_tag}"
}

function test_multus_connections() {
  EXEC='kubectl -n rook-ceph exec -t deploy/rook-ceph-tools -- ceph --connect-timeout 10'
  # daemons register their network addresses in the mon maps asynchronously
  # after the cluster reports ready (e.g., MDSes may not be in the fsmap yet),
  # so retry each check until it converges
  # each OSD should exist on both public and cluster network
  timeout 120 bash -c "until $EXEC osd dump | grep osd.0 | grep '192.168.20.' | grep '192.168.21.'; do sleep 5; done"
  # MDSes should exist on public network and NOT on cluster network
  timeout 120 bash -c "until $EXEC fs dump | grep myfs-a | grep '192.168.20.' | grep -v '192.168.21.'; do sleep 5; done"
}

function create_operator_toolbox() {
  cd "${REPO_DIR}/deploy/examples"
  sed -i "s|image: docker.io/rook/ceph:.*|image: docker.io/rook/ceph:local-build|g" toolbox-operator-image.yaml
  kubectl create -f toolbox-operator-image.yaml
}

function test_csi_rbd_workload {
  cd "${REPO_DIR}/deploy/examples/csi/rbd"
  sed -i 's|size: 3|size: 1|g' storageclass.yaml
  sed -i 's|requireSafeReplicaSize: true|requireSafeReplicaSize: false|g' storageclass.yaml
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/rook-ceph.rbd.csi.ceph.com-nodeplugin -c csi-rbdplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=rook-ceph.rbd.csi.ceph.com-nodeplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csirbd-demo-pod -- ls -alh /var/lib/www/html/
}

function test_csi_cephfs_workload {
  cd "${REPO_DIR}/deploy/examples/csi/cephfs"
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/rook-ceph.cephfs.csi.ceph.com-nodeplugin -c csi-cephfsplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=rook-ceph.cephfs.csi.ceph.com-nodeplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csicephfs-demo-pod -- ls -alh /var/lib/www/html/
}

function test_csi_nfs_workload {
  cd "${REPO_DIR}/deploy/examples/csi/nfs"
  sed -i "s|#- debug|- nolock|" storageclass.yaml
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 90 sh -c 'until kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=rook-ceph.nfs.csi.ceph.com-nodeplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csinfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csinfs-demo-pod -- ls -alh /var/lib/www/html/
}

function test_csi_nvmeof_workload {
  cd "${REPO_DIR}/deploy/examples/csi/nvmeof"

  local old_gateway_pod

  # Ensure CephCluster is fully ready before proceeding with NVMe-oF setup.
  # The ceph-csi-operator needs a valid CephConnection (populated from cluster info)
  # to bring the NVMe-oF controller plugin deployment to Available.
  kubectl -n rook-ceph wait --for=jsonpath='{.status.state}'=Created \
    cephcluster/my-cluster --timeout=600s

  sed -i 's/failureDomain: .*/failureDomain: osd/' nvmeof-pool.yaml
  sed -i 's/size: .*/size: 1/' nvmeof-pool.yaml
  kubectl create -f nvmeof-pool.yaml
  wait_for cephblockpool nvmeof rook-ceph 600

  kubectl create -f "${REPO_DIR}/deploy/examples/nvmeof-test.yaml"
  wait_for cephnvmeofgateway nvmeof rook-ceph 600
  timeout 300 bash <<EOF
until kubectl -n rook-ceph get pod --no-headers 2>/dev/null | awk '/rook-ceph-nvmeof-nvmeof-a-/ && \$3 == "Running" {f=1} END {exit !f}'; do
  echo "waiting for NVMe-oF gateway pod to be running"
  sleep 5
done
EOF

  # ceph-csi-operator creates these deployments, make sure they are available

  timeout 300 bash <<EOF
until kubectl -n rook-ceph get deployment --no-headers 2>/dev/null | grep -q "nvmeof"; do
  echo "waiting for CSI operator to create NVMe-oF controller plugin deployment"
  sleep 5
done
EOF

  # TODO: remove this diagnostic block once we confirm the root cause of the
  # NVMe-oF controller plugin timeout (see canary-integration-suite CI failure).
  if ! kubectl -n rook-ceph wait --for=condition=available \
    deployment/rook-ceph.nvmeof.csi.ceph.com-ctrlplugin --timeout=600s; then
    echo "=== NVMe-oF ctrlplugin deployment did not become available — collecting diagnostics ==="
    kubectl -n rook-ceph describe deployment/rook-ceph.nvmeof.csi.ceph.com-ctrlplugin || true
    kubectl -n rook-ceph get pods -l app=rook-ceph.nvmeof.csi.ceph.com-ctrlplugin -o wide || true
    kubectl -n rook-ceph describe pods -l app=rook-ceph.nvmeof.csi.ceph.com-ctrlplugin || true
    kubectl -n rook-ceph logs deploy/ceph-csi-controller-manager --tail=200 || true
    kubectl -n rook-ceph get cephconnection -o yaml || true
    kubectl -n rook-ceph get events --sort-by=.metadata.creationTimestamp | tail -80 || true
    kubectl -n rook-ceph get cephcluster -o yaml || true
    return 1
  fi

  timeout 300 bash <<EOF
until kubectl -n rook-ceph get daemonset --no-headers 2>/dev/null | grep -q "nvmeof"; do
  echo "waiting for CSI operator to create NVMe-oF node plugin daemonset"
  sleep 5
done
EOF

  kubectl -n rook-ceph rollout status \
    daemonset/rook-ceph.nvmeof.csi.ceph.com-nodeplugin --timeout=600s

  lsmod | grep -E 'nvme(_|-)(tcp|fabrics)' || true
  if [[ ! -d /sys/module/nvme_tcp ]]; then
    echo "nvme_tcp kernel module is required but unavailable on kernel $(uname -r)"
    return 1
  fi

  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  if ! kubectl wait pvc/nvmeof-external-volume --for=jsonpath='{.status.phase}'=Bound --timeout=300s; then
    echo "PVC nvmeof-external-volume did not reach Bound; collecting diagnostics..."
    kubectl describe pvc nvmeof-external-volume || true
    kubectl get events -n default --sort-by=.metadata.creationTimestamp | tail -n 50 || true
    kubectl -n rook-ceph logs -l "app.kubernetes.io/part-of=rook-ceph.nvmeof.csi.ceph.com" --tail=200 || true
    return 1
  fi

  kubectl create -f pod.yaml
  kubectl wait pod/nvmeof-test-pod --for=condition=Ready --timeout=300s

  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "echo 'abcd' > /mnt/nvmeof/test.txt && cat /mnt/nvmeof/test.txt"
  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "grep -qx 'abcd' /mnt/nvmeof/test.txt"

  old_gateway_pod="$(kubectl -n rook-ceph get pod --no-headers | awk '/rook-ceph-nvmeof-nvmeof-a-/ {if (!f) f=$1} END {print f}')"
  kubectl -n rook-ceph delete pod "${old_gateway_pod}"
  timeout 300 bash <<EOF
until new_gateway_pod=\$(kubectl -n rook-ceph get pod --no-headers | awk '/rook-ceph-nvmeof-nvmeof-a-/ && \$3 == "Running" {if (!f) f=\$1} END {print f}') && [[ -n "\${new_gateway_pod}" && "\${new_gateway_pod}" != "${old_gateway_pod}" ]]; do
  echo "waiting for NVMe-oF gateway pod restart"
  sleep 5
done
EOF

  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "grep -qx 'abcd' /mnt/nvmeof/test.txt"
  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "echo 'abcd efg' > /mnt/nvmeof/test.txt && grep -qx 'abcd efg' /mnt/nvmeof/test.txt"

  kubectl delete pod nvmeof-test-pod --grace-period=0 --force
  kubectl wait --for=delete pod/nvmeof-test-pod --timeout=180s

  kubectl create -f pod.yaml
  kubectl wait pod/nvmeof-test-pod --for=condition=Ready --timeout=300s
  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "grep -qx 'abcd efg' /mnt/nvmeof/test.txt"
  kubectl -n default exec pod/nvmeof-test-pod -- sh -c "echo 'abcd efg hij' > /mnt/nvmeof/test.txt && grep -qx 'abcd efg hij' /mnt/nvmeof/test.txt"
}

function toolbox() {
  kubectl -n rook-ceph exec -it "$(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}')" -- "$@"
}

function ceph() {
  toolbox ceph "$@"
}

function rbd() {
  toolbox rbd "$@"
}

function radosgw-admin() {
  toolbox radosgw-admin "$@"
}

function test_object_separate_pools() {
  expected_pools=(
    .mgr
    .rgw.root
    object-separate-pools.rgw.control
    object-separate-pools.rgw.meta
    object-separate-pools.rgw.log
    object-separate-pools.rgw.buckets.index
    object-separate-pools.rgw.buckets.non-ec
    object-separate-pools.rgw.otp
    object-separate-pools.rgw.buckets.data
  )

  output=$(ceph osd pool ls)
  readarray -t live_pools < <(printf '%s' "$output")

  errors=0
  for l in "${live_pools[@]}"; do
    found=false
    for e in "${expected_pools[@]}"; do
      if [[ "$l" == "$e" ]]; then
        found=true
        break
      fi
    done
    if [[ "$found" == false ]]; then
      echo "Live pool $l is not an expected pool"
      errors=$((errors + 1))
    fi
  done

  if [[ $errors -gt 0 ]]; then
    echo "Found $errors errors"
    exit $errors
  fi
}

function delete_cluster() {
  kubectl --namespace rook-ceph patch cephcluster rook-ceph --type merge -p '{"spec":{"cleanupPolicy":{"confirmation":"yes-really-destroy-data"}}}'
  kubectl --namespace rook-ceph delete cephcluster rook-ceph
  kubectl --namespace rook-ceph logs deploy/rook-ceph-operator
  wait_for_cleanup_pod
  kubectl --namespace rook-ceph delete --ignore-not-found=true -f deploy/examples/operator.yaml
  kubectl --namespace rook-ceph delete clientprofile rook-ceph
  kubectl --namespace rook-ceph logs deploy/ceph-csi-controller-manager
  kubectl --namespace rook-ceph delete --ignore-not-found=true -f deploy/examples/csi-operator.yaml
  remove_cluster_prerequisites
}

function check_keys_exists() {
  toolbox=$(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}')
  for key in "${@}"; do
    if kubectl -n rook-ceph exec "$toolbox" -- ceph auth get "$key"; then
      echo "key '$key' exists"
    else
      echo "key '$key' not exists"
      exit 1
    fi
  done
}

function check_keys_not_exists() {
  toolbox=$(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}')
  for key in "${@}"; do
    if kubectl -n rook-ceph exec "$toolbox" -- ceph auth get "$key"; then
      echo "key '$key' exists"
      exit 1
    else
      echo "key '$key' not exists"
    fi
  done
}

FUNCTION="$1"
shift # remove function arg now that we've recorded it
# call the function with the remainder of the user-provided args
# -e, -E, and -o=pipefail will ensure this script returns a failure if a part of the function fails
$FUNCTION "$@"
