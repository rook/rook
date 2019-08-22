#!/bin/bash
set -e

##############
# VARIABLES #
#############
MON_SECRET_NAME=rook-ceph-mon
MON_SECRET_CLUSTER_NAME_KEYNAME=cluster-name
MON_SECRET_FSID_KEYNAME=fsid
MON_SECRET_ADMIN_KEYRING_KEYNAME=admin-secret
MON_SECRET_MON_KEYRING_KEYNAME=mon-secret
MON_ENDPOINT_CONFIGMAP_NAME=rook-ceph-mon-endpoints
ROOK_EXTERNAL_CLUSTER_NAME=$NAMESPACE
ROOK_EXTERNAL_MAX_MON_ID=2
ROOK_EXTERNAL_MAPPING={}
ROOK_EXTERNAL_MONITOR_SECRET=mon-secret

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
    if [ -z "$ROOK_EXTERNAL_ADMIN_SECRET" ]; then
        echo "Please populate the environment variable ROOK_EXTERNAL_ADMIN_SECRET"
        exit 1
    fi
    if [ -z "$ROOK_EXTERNAL_CEPH_MON_DATA" ]; then
        echo "Please populate the environment variable ROOK_EXTERNAL_CEPH_MON_DATA"
        exit 1
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

########
# MAIN #
########
checkEnvVars
importSecret
importConfigMap