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

set -xEe

: "${DAEMON_TO_VALIDATE:=${1}}"
if [ -z "$DAEMON_TO_VALIDATE" ]; then
  DAEMON_TO_VALIDATE=all
fi
OSD_COUNT=$2

#############
# FUNCTIONS #
#############
EXEC_COMMAND="kubectl -n rook-ceph exec $(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}') -- ceph --connect-timeout 3"

function wait_for_daemon () {
  timeout=90
  daemon_to_test=$1
  while [ $timeout -ne 0 ]; do
    if eval $daemon_to_test; then
      return 0
    fi
    sleep 1
    let timeout=timeout-1
  done
  echo "current status:"
  $EXEC_COMMAND -s

  return 1
}

function test_demo_mon {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq quorum")
}

function test_demo_mgr {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'mgr:'")
}

function test_demo_osd {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq \"$OSD_COUNT osds: $OSD_COUNT up.*, $OSD_COUNT in.*\"")
}

function test_demo_rgw {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'rgw:'")
}

function test_demo_mds {
  echo "Waiting for the MDS to be ready"
  # NOTE: metadata server always takes up to 5 sec to run
  # so we first check if the pools exit, from that we assume that
  # the process will start. We stop waiting after 10 seconds.
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND osd dump | grep -sq cephfs && $EXEC_COMMAND -s | grep -sq up")
}

function test_demo_rbd_mirror {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'rbd-mirror:'")
}

function test_demo_fs_mirror {
  # shellcheck disable=SC2046
    return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'cephfs-mirror:'")
}

function test_demo_pool {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq '11 pools'")
}

function test_csi {
  timeout 360 bash -x <<-'EOF'
    until [[ "$(kubectl -n rook-ceph get pods --field-selector=status.phase=Running|grep -c ^csi-)" -eq 6 ]]; do
      echo "waiting for csi pods to be ready with multus or pod networking"
      sleep 5
    done
    
    if [ -n "$IS_MULTUS" ]; then
      echo "verifying csi holder interfaces (multus ones must be present)"
      kubectl -n rook-ceph exec -t ds/csi-rbdplugin-holder-my-cluster -- grep net /proc/net/dev
      kubectl -n rook-ceph exec -t ds/csi-cephfsplugin-holder-my-cluster -- grep net /proc/net/dev
    fi
EOF
}

function wait_for_ceph_csi_configmap_to_be_updated {
  timeout 60 bash <<EOF
until [[ $(kubectl -n rook-ceph get configmap rook-ceph-csi-config  -o jsonpath="{.data.csi-cluster-config-json}" | jq .[0].rbd.netNamespaceFilePath) != "null" ]]; do
  echo "waiting for ceph csi configmap to be updated with rbd.netNamespaceFilePath"
  sleep 5
done
EOF
  timeout 60 bash <<EOF
until [[ $(kubectl -n rook-ceph get configmap rook-ceph-csi-config  -o jsonpath="{.data.csi-cluster-config-json}" | jq .[0].cephFS.netNamespaceFilePath) != "null" ]]; do
  echo "waiting for ceph csi configmap to be updated with cephFS.netNamespaceFilePath"
  sleep 5
done
EOF
}

function test_csi_rbd_workload {
  pushd deploy/examples/csi/rbd
  sed -i 's|size: 3|size: 1|g' storageclass.yaml
  sed -i 's|requireSafeReplicaSize: true|requireSafeReplicaSize: false|g' storageclass.yaml
  # Using kubectl apply to avoid already exists error as storageclass.yaml has the pool spec
  kubectl apply -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 45 sh -c 'until kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/csi-rbdplugin -c csi-rbdplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=csi-rbdplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csirbd-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csirbd-demo-pod -- ls -alh /var/lib/www/html/
  popd
}

function test_csi_cephfs_workload {
  pushd deploy/examples/csi/cephfs
  kubectl create -f storageclass.yaml
  kubectl create -f pvc.yaml
  kubectl create -f pod.yaml
  timeout 45 sh -c 'until kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test bs=1M count=1; do echo "waiting for test pod to be ready" && sleep 1; done'
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test oflag=direct bs=1M count=1
  kubectl -n rook-ceph logs ds/csi-cephfsplugin -c csi-cephfsplugin
  kubectl -n rook-ceph delete "$(kubectl -n rook-ceph get pod --selector=app=csi-cephfsplugin --field-selector=status.phase=Running -o name)"
  kubectl exec -t pod/csicephfs-demo-pod -- dd if=/dev/random of=/var/lib/www/html/test1 oflag=direct bs=1M count=1
  kubectl exec -t pod/csicephfs-demo-pod -- ls -alh /var/lib/www/html/
  popd
}

function test_nfs {
  timeout 360 bash <<-'EOF'
    until [[ "$(kubectl -n rook-ceph get pods --field-selector=status.phase=Running|grep -c ^rook-ceph-nfs-)" -eq 1 ]]; do
      echo "waiting for nfs pods to be ready"
      sleep 5
    done
EOF
}

function test_multus_osd {
  for i in $(seq 1 2); do
    kubectl -n rook-ceph exec -t deploy/rook-ceph-osd-0 -c osd -- grep net"$i" /proc/net/dev
    kubectl -n rook-ceph exec -t deploy/rook-ceph-osd-0 -c osd -- grep net"$i" /proc/net/dev
  done
}

########
# MAIN #
########
test_csi
wait_for_ceph_csi_configmap_to_be_updated
test_csi_rbd_workload
test_csi_cephfs_workload
test_demo_mon
test_demo_mgr

if [[ "$DAEMON_TO_VALIDATE" == "all" ]]; then
  daemons_list="osd mds rgw rbd_mirror fs_mirror nfs"
else
  # change commas to space
  comma_to_space=${DAEMON_TO_VALIDATE//,/ }

  # transform to an array
  IFS=" " read -r -a array <<< "$comma_to_space"

  # sort and remove potential duplicate
  daemons_list=$(echo "${array[@]}" | tr ' ' '\n' | sort -u | tr '\n' ' ')
fi

for daemon in $daemons_list; do
  case "$daemon" in
    mon)
      continue
      ;;
    mgr)
      continue
      ;;
    osd)
      test_demo_osd
      ;;
    osd_multus)
      test_demo_osd
      test_multus_osd
      ;;
    mds)
      test_demo_mds
      ;;
    rgw)
      test_demo_rgw
      ;;
    rbd_mirror)
      test_demo_rbd_mirror
      ;;
    fs_mirror)
      test_demo_fs_mirror
      ;;
    nfs)
      test_nfs
      ;;
    *)
      log "ERROR: unknown daemon to validate!"
      log "Available daemon are: mon mgr osd mds rgw rbd_mirror fs_mirror"
      exit 1
      ;;
  esac
done

echo "Ceph is up and running, have a look!"
$EXEC_COMMAND -s
kubectl -n rook-ceph get pods
kubectl -n rook-ceph logs "$(kubectl -n rook-ceph -l app=rook-ceph-operator get pods -o jsonpath='{.items[*].metadata.name}')"
kubectl -n rook-ceph get cephcluster -o yaml
