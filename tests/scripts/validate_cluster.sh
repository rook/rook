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
# The second script arg is optional and depends on the daemon
if [ "$DAEMON_TO_VALIDATE" == "rgw" ]; then
  export OBJECT_STORE_NAME=$2
else
  export OSD_COUNT=$2
  # default to the name of the object store from object-a.yaml
  export OBJECT_STORE_NAME=store-a
fi

#############
# FUNCTIONS #
#############
EXEC_COMMAND="kubectl -n rook-ceph exec $(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}') -- ceph --connect-timeout 10"

function wait_for_daemon() {
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
  ret_val=$(wait_for_daemon "$EXEC_COMMAND -s | grep -sq \"$OSD_COUNT osds: $OSD_COUNT up.*, $OSD_COUNT in.*\"")
  # debug info for an intermittent failure
  echo "Return value = $ret_val"
  return $ret_val
}

function test_demo_rgw {
  timeout 360 bash -x <<-'EOF'
    until [[ "$(kubectl -n rook-ceph get pods -l rgw=$OBJECT_STORE_NAME -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}')" == "True" ]]; do
      echo "waiting for rgw pods to be ready"
      sleep 5
    done
EOF
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
      echo "waiting for csi pods to be ready"
      sleep 5
    done
EOF
}

function test_nfs {
  timeout 360 bash <<-'EOF'
    until [[ "$(kubectl -n rook-ceph get pods --field-selector=status.phase=Running|grep -c ^rook-ceph-nfs-)" -eq 1 ]]; do
      echo "waiting for nfs pods to be ready"
      sleep 5
    done
EOF
}

########
# MAIN #
########
test_csi
test_demo_mon
test_demo_mgr

if [[ "$DAEMON_TO_VALIDATE" == "all" ]]; then
  daemons_list="osd mds rgw rbd_mirror fs_mirror nfs"
else
  # change commas to space
  comma_to_space=${DAEMON_TO_VALIDATE//,/ }

  # transform to an array
  IFS=" " read -r -a array <<<"$comma_to_space"

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
