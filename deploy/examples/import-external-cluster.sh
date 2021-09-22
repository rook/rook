#!/bin/bash
set -e

##############
# VARIABLES #
#############
MON_SECRET_NAME=rook-ceph-mon
CSI_RBD_NODE_SECRET_NAME=rook-csi-rbd-node
CSI_RBD_PROVISIONER_SECRET_NAME=rook-csi-rbd-provisioner
CSI_CEPHFS_NODE_SECRET_NAME=rook-csi-cephfs-node
CSI_CEPHFS_PROVISIONER_SECRET_NAME=rook-csi-cephfs-provisioner
RGW_ADMIN_OPS_USER_SECRET_NAME=rgw-admin-ops-user
MON_SECRET_CLUSTER_NAME_KEYNAME=cluster-name
MON_SECRET_FSID_KEYNAME=fsid
MON_SECRET_ADMIN_KEYRING_KEYNAME=admin-secret
MON_SECRET_MON_KEYRING_KEYNAME=mon-secret
MON_SECRET_CEPH_USERNAME_KEYNAME=ceph-username
MON_SECRET_CEPH_SECRET_KEYNAME=ceph-secret
MON_ENDPOINT_CONFIGMAP_NAME=rook-ceph-mon-endpoints
ROOK_EXTERNAL_CLUSTER_NAME=$NAMESPACE
ROOK_EXTERNAL_MAX_MON_ID=2
ROOK_EXTERNAL_MAPPING={}
ROOK_EXTERNAL_MONITOR_SECRET=mon-secret
: "${ROOK_EXTERNAL_ADMIN_SECRET:=admin-secret}"

#############
# FUNCTIONS #
#############

function checkEnvVars() {
  if [ -z "$NAMESPACE" ]; then
    echo "Please populate the environment variable NAMESPACE"
    exit 1
  fi
  if [ -z "$ROOK_EXTERNAL_FSID" ]; then
    echo "Please populate the environment variable ROOK_EXTERNAL_FSID"
    exit 1
  fi
  if [ -z "$ROOK_EXTERNAL_CEPH_MON_DATA" ]; then
    echo "Please populate the environment variable ROOK_EXTERNAL_CEPH_MON_DATA"
    exit 1
  fi
  if [[ "$ROOK_EXTERNAL_ADMIN_SECRET" == "admin-secret" ]]; then
    if [ -z "$ROOK_EXTERNAL_USER_SECRET" ]; then
      echo "Please populate the environment variable ROOK_EXTERNAL_USER_SECRET"
      exit 1
    fi
    if [ -z "$ROOK_EXTERNAL_USERNAME" ]; then
      echo "Please populate the environment variable ROOK_EXTERNAL_USERNAME"
      exit 1
    fi
    if [ -z "$CSI_RBD_NODE_SECRET" ]; then
      echo "Please populate the environment variable CSI_RBD_NODE_SECRET"
      exit 1
    fi
    if [ -z "$CSI_RBD_PROVISIONER_SECRET" ]; then
      echo "Please populate the environment variable CSI_RBD_PROVISIONER_SECRET"
      exit 1
    fi
    if [ -z "$CSI_CEPHFS_NODE_SECRET" ]; then
      echo "Please populate the environment variable CSI_CEPHFS_NODE_SECRET"
      exit 1
    fi
    if [ -z "$CSI_CEPHFS_PROVISIONER_SECRET" ]; then
      echo "Please populate the environment variable CSI_CEPHFS_PROVISIONER_SECRET"
      exit 1
    fi
  fi
  if [[ "$ROOK_EXTERNAL_ADMIN_SECRET" != "admin-secret" ]] && [ -n "$ROOK_EXTERNAL_USER_SECRET" ] ; then
    echo "Providing both ROOK_EXTERNAL_ADMIN_SECRET and ROOK_EXTERNAL_USER_SECRET is not supported, choose one only."
    exit 1
  fi
}

function importSecret() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$MON_SECRET_NAME" \
  --from-literal="$MON_SECRET_CLUSTER_NAME_KEYNAME"="$ROOK_EXTERNAL_CLUSTER_NAME" \
  --from-literal="$MON_SECRET_FSID_KEYNAME"="$ROOK_EXTERNAL_FSID" \
  --from-literal="$MON_SECRET_ADMIN_KEYRING_KEYNAME"="$ROOK_EXTERNAL_ADMIN_SECRET" \
  --from-literal="$MON_SECRET_MON_KEYRING_KEYNAME"="$ROOK_EXTERNAL_MONITOR_SECRET" \
  --from-literal="$MON_SECRET_CEPH_USERNAME_KEYNAME"="$ROOK_EXTERNAL_USERNAME" \
  --from-literal="$MON_SECRET_CEPH_SECRET_KEYNAME"="$ROOK_EXTERNAL_USER_SECRET"
}

function importConfigMap() {
  kubectl -n "$NAMESPACE" \
  create \
  configmap \
  "$MON_ENDPOINT_CONFIGMAP_NAME" \
  --from-literal=data="$ROOK_EXTERNAL_CEPH_MON_DATA" \
  --from-literal=mapping="$ROOK_EXTERNAL_MAPPING" \
  --from-literal=maxMonId="$ROOK_EXTERNAL_MAX_MON_ID"
}

function importCsiRBDNodeSecret() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$CSI_RBD_NODE_SECRET_NAME" \
  --from-literal=userID=csi-rbd-node \
  --from-literal=userKey="$CSI_RBD_NODE_SECRET"
}

function importCsiRBDProvisionerSecret() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$CSI_RBD_PROVISIONER_SECRET_NAME" \
  --from-literal=userID=csi-rbd-provisioner \
  --from-literal=userKey="$CSI_RBD_PROVISIONER_SECRET"
}

function importCsiCephFSNodeSecret() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$CSI_CEPHFS_NODE_SECRET_NAME" \
  --from-literal=adminID=csi-cephfs-node \
  --from-literal=adminKey="$CSI_CEPHFS_NODE_SECRET"
}

function importCsiCephFSProvisionerSecret() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$CSI_CEPHFS_PROVISIONER_SECRET_NAME" \
  --from-literal=adminID=csi-cephfs-provisioner \
  --from-literal=adminKey="$CSI_CEPHFS_PROVISIONER_SECRET"
}

function importRGWAdminOpsUser() {
  kubectl -n "$NAMESPACE" \
  create \
  secret \
  generic \
  --type="kubernetes.io/rook" \
  "$RGW_ADMIN_OPS_USER_SECRET_NAME" \
  --from-literal=accessKey="$RGW_ADMIN_OPS_USER_ACCESS_KEY" \
  --from-literal=secretKey="$RGW_ADMIN_OPS_USER_SECRET_KEY"
}

########
# MAIN #
########
checkEnvVars
importSecret
importConfigMap
importCsiRBDNodeSecret
importCsiRBDProvisionerSecret
importCsiCephFSNodeSecret
importCsiCephFSProvisionerSecret
importRGWAdminOpsUser
