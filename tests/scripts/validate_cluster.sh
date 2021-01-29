#!/usr/bin/env bash
set -xE

: "${DAEMON_TO_VALIDATE:=${1}}"
if [ -z "$DAEMON_TO_VALIDATE" ]; then
  DAEMON_TO_VALIDATE=all
fi


#############
# FUNCTIONS #
#############
EXEC_COMMAND="kubectl -n rook-ceph exec $(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[0].metadata.name}') -- ceph --connect-timeout 3"

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
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq '1 osds: 1 up.*, 1 in.*'")
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
  return $(wait_for_daemon "$EXEC_COMMAND osd dump | grep -sq cephfs && $EXEC_COMMAND -s | grep -sq 'up:active'")
}

function test_demo_rgw {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'rgw:'")
}

function test_demo_rbd_mirror {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq 'rbd-mirror:'")
}

function test_demo_fs_mirror {
  # shellcheck disable=SC2046
  timeout 90 sh -c 'until [ $(kubectl -n rook-ceph get pods --field-selector=status.phase=Running -l app=rook-ceph-filesystem-mirror --no-headers=true|wc -l) -eq 1 ]; do sleep 1; done'
  if [ $? -eq 0 ]; then
    return 0
  fi
  return 1
}

function test_demo_pool {
  # shellcheck disable=SC2046
  return $(wait_for_daemon "$EXEC_COMMAND -s | grep -sq '11 pools'")
}

function test_csi {
  # shellcheck disable=SC2046
  timeout 90 sh -c 'until [ $(kubectl -n rook-ceph get pods --field-selector=status.phase=Running|grep -c ^csi-) -eq 4 ]; do sleep 1; done'
  if [ $? -eq 0 ]; then
    return 0
  fi
  return 1
}

function display_status {
  set +x
  $EXEC_COMMAND -s > test/ceph-status.txt
  $EXEC_COMMAND osd dump > test/osd-dump.txt

  kubectl -n rook-ceph logs "$(kubectl -n rook-ceph -l app=rook-ceph-operator get pods -o jsonpath='{.items[*].metadata.name}')" > test/operator.txt
  kubectl -n rook-ceph get pods > test/pods-list.txt
  kubectl -n rook-ceph describe job/"$(kubectl -n rook-ceph get pod -l app=rook-ceph-osd-prepare -o jsonpath='{.items[*].metadata.name}')" > test/osd-prepare.txt
  kubectl -n rook-ceph describe deploy/rook-ceph-osd-0 > test/osd-deploy.txt
  kubectl get all -n rook-ceph -o wide > test/cluster-wide.txt
  kubectl get all -n rook-ceph -o yaml > test/cluster-yaml.txt
  kubectl -n rook-ceph get cephcluster -o yaml > test/cephcluster.txt
  sudo lsblk > test/lsblk.txt
  set -x
}

########
# MAIN #
########
test_csi
test_demo_mon
test_demo_mgr

if [[ "$DAEMON_TO_VALIDATE" == "all" ]]; then
  daemons_list="osd mds rgw rbd_mirror"
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
    mds)
      test_demo_mds
      ;;
    rgw)
      test_demo_rgw
      ;;
    rbd_mirror)
      test_demo_rbd_mirror
      ;;
    *)
      log "ERROR: unknown daemon to validate!"
      log "Available daemon are: mon mgr osd mds rgw rbd_mirror"
      exit 1
      ;;
  esac
done

display_status

echo "Ceph is up and running, have a look!"
$EXEC_COMMAND -s
kubectl -n rook-ceph get pods
kubectl -n rook-ceph logs "$(kubectl -n rook-ceph -l app=rook-ceph-operator get pods -o jsonpath='{.items[*].metadata.name}')"
kubectl -n rook-ceph get cephcluster -o yaml
