#!/bin/bash
set -e

##############
# VARIABLES #
#############
MON_SECRET_NAME=rook-ceph-mon
OPERATOR_CREDS=rook-ceph-operator-creds
CSI_RBD_NODE_SECRET_NAME=rook-csi-rbd-node
CSI_RBD_PROVISIONER_SECRET_NAME=rook-csi-rbd-provisioner
CSI_CEPHFS_NODE_SECRET_NAME=rook-csi-cephfs-node
CSI_CEPHFS_PROVISIONER_SECRET_NAME=rook-csi-cephfs-provisioner
MON_SECRET_CLUSTER_NAME_KEYNAME=cluster-name
MON_SECRET_FSID_KEYNAME=fsid
MON_SECRET_ADMIN_KEYRING_KEYNAME=admin-secret
MON_SECRET_MON_KEYRING_KEYNAME=mon-secret
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
        if [ -z "$CSI_RBD_NODE_SECRET_SECRET" ]; then
            echo "Please populate the environment variable CSI_RBD_NODE_SECRET_SECRET"
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
}

function importSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$MON_SECRET_NAME" \
    --from-literal="$MON_SECRET_CLUSTER_NAME_KEYNAME"="$ROOK_EXTERNAL_CLUSTER_NAME" \
    --from-literal="$MON_SECRET_FSID_KEYNAME"="$ROOK_EXTERNAL_FSID" \
    --from-literal="$MON_SECRET_ADMIN_KEYRING_KEYNAME"="$ROOK_EXTERNAL_ADMIN_SECRET" \
    --from-literal="$MON_SECRET_MON_KEYRING_KEYNAME"="$ROOK_EXTERNAL_MONITOR_SECRET"
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

function importCheckerSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$OPERATOR_CREDS" \
    --from-literal=userID="$ROOK_EXTERNAL_USERNAME" \
    --from-literal=userKey="$ROOK_EXTERNAL_USER_SECRET"
}

function importCsiRBDNodeSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$CSI_RBD_NODE_SECRET_NAME" \
    --from-literal=userID=csi-rbd-node \
    --from-literal=userKey="$CSI_RBD_NODE_SECRET_SECRET"
}

function importCsiRBDProvisionerSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$CSI_RBD_PROVISIONER_SECRET_NAME" \
    --from-literal=userID=csi-rbd-provisioner \
    --from-literal=userKey="$CSI_RBD_PROVISIONER_SECRET"
}

function importCsiCephFSNodeSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$CSI_CEPHFS_NODE_SECRET_NAME" \
    --from-literal=userID=csi-cephfs-node \
    --from-literal=userKey="$CSI_CEPHFS_NODE_SECRET"
}

function importCsiCephFSProvisionerSecret() {
    kubectl -n "$NAMESPACE" \
    create \
    secret \
    generic \
    "$CSI_CEPHFS_PROVISIONER_SECRET_NAME" \
    --from-literal=userID=csi-cephfs-provisioner \
    --from-literal=userKey="$CSI_CEPHFS_PROVISIONER_SECRET"
}

########
# MAIN #
########
checkEnvVars
importSecret
importConfigMap
importCheckerSecret
importCsiRBDNodeSecret
importCsiRBDProvisionerSecret
importCsiCephFSNodeSecret
importCsiCephFSProvisionerSecret
