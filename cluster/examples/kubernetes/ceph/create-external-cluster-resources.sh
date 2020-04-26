#!/bin/bash
# this script creates all the users/keys on the external cluster
# those keys will be injected via the import-external-cluster.sh once this one is done running
# so you can run import-external-cluster.sh right after this script
set -Eeuo pipefail

#############
# VARIABLES #
#############

: "${CLIENT_CHECKER_NAME:=client.healthchecker}"

#############
# FUNCTIONS #
#############

function is_available {
  command -v "$@" &>/dev/null
}

function checkEnv() {
  if ! is_available ceph; then
    echo "'ceph' binary is expected'"
    exit 1
  fi

  if ! ceph -s 1>/dev/null; then
    echo "cannot connect to the ceph cluster"
    exit 1
fi
}

function createCheckerKey() {
  checkerKey=$(ceph auth get-or-create "$CLIENT_CHECKER_NAME" mon 'allow r, allow command quorum_status'|awk '/key =/ { print $3}')
  echo "export ROOK_EXTERNAL_USER_SECRET=$checkerKey"
  echo "export ROOK_EXTERNAL_USERNAME=$CLIENT_CHECKER_NAME"
}

function createCephCSIKeyringRBDNode() {
  cephCSIKeyringRBDNodeKey=$(ceph auth get-or-create client.csi-rbd-node mon 'profile rbd' osd 'profile rbd'|awk '/key =/ { print $3}')
  echo "export CSI_RBD_NODE_SECRET_SECRET=$cephCSIKeyringRBDNodeKey"
}

function createCephCSIKeyringRBDProvisioner() {
  cephCSIKeyringRBDProvisionerKey=$(ceph auth get-or-create client.csi-rbd-provisioner mon 'profile rbd' mgr 'allow rw' osd 'profile rbd'|awk '/key =/ { print $3}')
  echo "export CSI_RBD_PROVISIONER_SECRET=$cephCSIKeyringRBDProvisionerKey"
}

function createCephCSIKeyringCephFSNode() {
  cephCSIKeyringCephFSNodeKey=$(ceph auth get-or-create client.csi-cephfs-node mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs *=*' mds 'allow rw'|awk '/key =/ { print $3}')
  echo "export CSI_CEPHFS_NODE_SECRET=$cephCSIKeyringCephFSNodeKey"
}

function createCephCSIKeyringCephFSProvisioner() {
  cephCSIKeyringCephFSProvisionerKey=$(ceph auth get-or-create client.csi-cephfs-provisioner mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs metadata=*'|awk '/key =/ { print $3}')
  echo "export CSI_CEPHFS_PROVISIONER_SECRET=$cephCSIKeyringCephFSProvisionerKey"
}

function getCephExternalFSID() {
  cephFSID=$(ceph fsid)
  echo "export ROOK_EXTERNAL_FSID=$cephFSID"
}

function getCephExternalMonData() {
  cephQuorumOp="$(ceph quorum_status)"
  # fetch the quorum leader name
  cephQuorumLeaderName="$(echo $cephQuorumOp |sed -n 's@.*"quorum_leader_name":"\([^"]*\)".*@\1@gp')"
  # get the <ip_address>:<port> of the quorum leader
  # (and remove any '/<digit>' from the tail end of the output)
  cephQuorumLeaderIP="$(echo $cephQuorumOp |sed -n -r 's@.*"name":"'$cephQuorumLeaderName'",(.*)@\1@gp' |sed -r -n 's@"public_addr":"([^"]*)".*|.@\1@gp' |sed 's@/.*@@g')"
  rookExternalMonData="$cephQuorumLeaderName=$cephQuorumLeaderIP"
  echo "export ROOK_EXTERNAL_MON_DATA='$rookExternalMonData'"
}

function generateJSON() {
  local outFile=$1
  [ -z "$outFile" ] && outFile=/dev/null
  cat <<EOF >$outFile
{
  "CSI_CEPHFS_PROVISIONER_SECRET": "$cephCSIKeyringCephFSProvisionerKey",
  "CSI_CEPHFS_NODE_SECRET": "$cephCSIKeyringCephFSNodeKey",
  "CSI_RBD_PROVISIONER_SECRET": "$cephCSIKeyringRBDProvisionerKey",
  "CSI_RBD_NODE_SECRET_SECRET": "$cephCSIKeyringRBDNodeKey",
  "ROOK_EXTERNAL_USERNAME": "$CLIENT_CHECKER_NAME",
  "ROOK_EXTERNAL_USER_SECRET": "$checkerKey",
  "ROOK_EXTERNAL_FSID": "$cephFSID",
  "ROOK_EXTERNAL_MON_DATA": "$rookExternalMonData"
}
EOF
}

function processJSONArgs() {
  local jsonArg="$(echo $* |sed -n -r 's@.*[[:space:]]*(--\<json\>)[[:space:]]*.*@\1@gp')"
  # if there is no JSON arg return silently
  [ -z "$jsonArg" ] && return
  local jsonFile="$(echo $* |sed -n -r 's@.*[[:space:]]*--json[[:space:]]+([^ ]*).*@\1@gp')"
  generateJSON "$jsonFile"
}

########
# MAIN #
########
checkEnv
createCheckerKey
createCephCSIKeyringRBDNode
createCephCSIKeyringRBDProvisioner
createCephCSIKeyringCephFSNode
createCephCSIKeyringCephFSProvisioner
getCephExternalFSID
getCephExternalMonData

processJSONArgs $*

echo -e "# successfully created users and keys, execute the above commands and run import-external-cluster.sh to inject them in your Kubernetes cluster."
