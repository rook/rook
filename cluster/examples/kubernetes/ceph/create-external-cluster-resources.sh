#!/bin/bash
# this script creates all the users/keys on the external cluster
# those keys will be injected via the import-external-cluster.sh once this one is done running
# so you can run import-external-cluster.sh right after this script
set -Eeuo pipefail

#############
# VARIABLES #
#############

: "${CLIENT_CHECKER_NAME:=client.healthchecker}"
: "${RGW_POOL_PREFIX:=default}"

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
  checkerKey=$(ceph auth get-or-create "$CLIENT_CHECKER_NAME" mon 'allow r, allow command quorum_status' mgr 'allow command config' osd 'allow rwx pool='"$RGW_POOL_PREFIX"'.rgw.meta, allow r pool=.rgw.root, allow rw pool='"$RGW_POOL_PREFIX"'.rgw.control, allow x pool='"$RGW_POOL_PREFIX"'.rgw.buckets.index, allow x pool='"$RGW_POOL_PREFIX"'.rgw.log'|awk '/key =/ { print $3}')
  echo "export ROOK_EXTERNAL_USER_SECRET=$checkerKey"
  echo "export ROOK_EXTERNAL_USERNAME=$CLIENT_CHECKER_NAME"
}

function createCephCSIKeyringRBDNode() {
  cephCSIKeyringRBDNodeKey=$(ceph auth get-or-create client.csi-rbd-node mon 'profile rbd' osd 'profile rbd'|awk '/key =/ { print $3}')
  echo "export CSI_RBD_NODE_SECRET=$cephCSIKeyringRBDNodeKey"
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


########
# MAIN #
########
checkEnv
createCheckerKey
createCephCSIKeyringRBDNode
createCephCSIKeyringRBDProvisioner
createCephCSIKeyringCephFSNode
createCephCSIKeyringCephFSProvisioner

echo -e "successfully created users and keys, execute the above commands and run import-external-cluster.sh to inject them in your Kubernetes cluster."
