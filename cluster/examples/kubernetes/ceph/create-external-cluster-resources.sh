#!/bin/bash
# this script creates all the users/keys on the external cluster
# those keys will be injected via the import-external-cluster.sh once this one is done running
# so you can run import-external-cluster.sh right after this script
# run me like: . cluster/examples/kubernetes/ceph/create-external-cluster-resources.sh
set -e

#############
# VARIABLES #
#############

: "${CLIENT_CHECKER_NAME:=client.healthchecker}"
: "${RGW_POOL_PREFIX:=default}"
: "${ns:=rook-ceph-external}"

#############
# FUNCTIONS #
#############

function is_available {
  command -v "$@" &>/dev/null
}

function checkEnv() {
  if ! is_available ceph; then
    echo "'ceph' binary is expected'"
    return 1
  fi
  
  if ! is_available jq; then
    echo "'jq' binary is expected'"
    return 1
  fi
  
  if ! ceph -s 1>/dev/null; then
    echo "cannot connect to the ceph cluster"
    return 1
  fi
}

function createCheckerKey() {
  ROOK_EXTERNAL_USER_SECRET=$(ceph auth get-or-create "$CLIENT_CHECKER_NAME" mon 'allow r, allow command quorum_status' mgr 'allow command config' osd 'allow rwx pool='"$RGW_POOL_PREFIX"'.rgw.meta, allow r pool=.rgw.root, allow rw pool='"$RGW_POOL_PREFIX"'.rgw.control, allow x pool='"$RGW_POOL_PREFIX"'.rgw.buckets.index, allow x pool='"$RGW_POOL_PREFIX"'.rgw.log'|awk '/key =/ { print $3}')
  export ROOK_EXTERNAL_USER_SECRET
  export ROOK_EXTERNAL_USERNAME=$CLIENT_CHECKER_NAME
}

function createCephCSIKeyringRBDNode() {
  CSI_RBD_NODE_SECRET=$(ceph auth get-or-create client.csi-rbd-node mon 'profile rbd' osd 'profile rbd'|awk '/key =/ { print $3}')
  export CSI_RBD_NODE_SECRET
}

function createCephCSIKeyringRBDProvisioner() {
  CSI_RBD_PROVISIONER_SECRET=$(ceph auth get-or-create client.csi-rbd-provisioner mon 'profile rbd' mgr 'allow rw' osd 'profile rbd'|awk '/key =/ { print $3}')
  export CSI_RBD_PROVISIONER_SECRET
}

function createCephCSIKeyringCephFSNode() {
  CSI_CEPHFS_NODE_SECRET=$(ceph auth get-or-create client.csi-cephfs-node mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs *=*' mds 'allow rw'|awk '/key =/ { print $3}')
  export CSI_CEPHFS_NODE_SECRET
}

function createCephCSIKeyringCephFSProvisioner() {
  CSI_CEPHFS_PROVISIONER_SECRET=$(ceph auth get-or-create client.csi-cephfs-provisioner mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs metadata=*'|awk '/key =/ { print $3}')
  export CSI_CEPHFS_PROVISIONER_SECRET
}

function getFSID() {
  ROOK_EXTERNAL_FSID=$(ceph fsid)
  export ROOK_EXTERNAL_FSID
}

function externalMonData() {
  ROOK_EXTERNAL_CEPH_MON_DATA=$(ceph mon dump -f json 2>/dev/null|jq --raw-output .mons[0].name)=$(ceph mon dump -f json 2>/dev/null|jq --raw-output .mons[0].public_addrs.addrvec[0].addr)
  export ROOK_EXTERNAL_CEPH_MON_DATA
}

function namespace() {
  export NAMESPACE=$ns
}

function createRGWAdminOpsUser() {
  createRGWAdminOpsUserKeys=$(radosgw-admin user create --uid rgw-admin-ops-user --display-name "Rook RGW Admin Ops user" --caps "buckets=*;users=*;usage=read;metadata=read;zone=read"|jq --raw-output .keys[0])
  createRGWAdminOpsUserAccessKey=$(echo "$createRGWAdminOpsUserKeys"|jq --raw-output .access_key)
  createRGWAdminOpsUserSecretKey=$(echo "$createRGWAdminOpsUserKeys"|jq --raw-output .secret_key)
  echo "export RGW_ADMIN_OPS_USER_ACCESS_KEY=$createRGWAdminOpsUserAccessKey"
  echo "export RGW_ADMIN_OPS_USER_SECRET_KEY=$createRGWAdminOpsUserSecretKey"
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
getFSID
externalMonData
namespace
createRGWAdminOpsUser
