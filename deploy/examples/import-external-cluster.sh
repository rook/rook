#!/usr/bin/env -S bash
set -e

##############
# VARIABLES #
#############
NAMESPACE=${NAMESPACE:="rook-ceph-external"}
MON_SECRET_NAME=rook-ceph-mon
RGW_ADMIN_OPS_USER_SECRET_NAME=rgw-admin-ops-user
MON_SECRET_CLUSTER_NAME_KEYNAME=cluster-name
MON_SECRET_FSID_KEYNAME=fsid
MON_SECRET_ADMIN_KEYRING_KEYNAME=admin-secret
MON_SECRET_MON_KEYRING_KEYNAME=mon-secret
MON_SECRET_CEPH_USERNAME_KEYNAME=ceph-username
MON_SECRET_CEPH_SECRET_KEYNAME=ceph-secret
MON_ENDPOINT_CONFIGMAP_NAME=rook-ceph-mon-endpoints
ROOK_EXTERNAL_CLUSTER_NAME=$NAMESPACE
ROOK_RBD_FEATURES=${ROOK_RBD_FEATURES:-"layering"}
ROOK_EXTERNAL_MAX_MON_ID=2
ROOK_EXTERNAL_MAPPING={}
RBD_STORAGE_CLASS_NAME=ceph-rbd
CEPHFS_STORAGE_CLASS_NAME=cephfs
ROOK_EXTERNAL_MONITOR_SECRET=mon-secret
OPERATOR_NAMESPACE=rook-ceph                                 # default set to rook-ceph
RBD_PROVISIONER=$OPERATOR_NAMESPACE".rbd.csi.ceph.com"       # driver:namespace:operator
CEPHFS_PROVISIONER=$OPERATOR_NAMESPACE".cephfs.csi.ceph.com" # driver:namespace:operator
CLUSTER_ID_RBD=$NAMESPACE
CLUSTER_ID_CEPHFS=$NAMESPACE
: "${ROOK_EXTERNAL_ADMIN_SECRET:=admin-secret}"

#############
# FUNCTIONS #
#############

function checkEnvVars() {
  if [ -z "$NAMESPACE" ]; then
    echo "Please populate the environment variable NAMESPACE"
    exit 1
  fi
  if [ -z "$ROOK_RBD_FEATURES" ] || [[ ! "$ROOK_RBD_FEATURES" =~ .*"layering".* ]]; then
    echo "Please populate the environment variable ROOK_RBD_FEATURES"
    echo "For a kernel earlier than 5.4 use a value of 'layering'; for 5.4 or later"
    echo "use 'layering,fast-diff,object-map,deep-flatten,exclusive-lock'"
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
  fi
  if [[ "$ROOK_EXTERNAL_ADMIN_SECRET" != "admin-secret" ]] && [ -n "$ROOK_EXTERNAL_USER_SECRET" ]; then
    echo "Providing both ROOK_EXTERNAL_ADMIN_SECRET and ROOK_EXTERNAL_USER_SECRET is not supported, choose one only."
    exit 1
  fi
}

function createClusterNamespace() {
  if ! kubectl get namespace "$NAMESPACE" &>/dev/null; then
    kubectl \
      create \
      namespace \
      "$NAMESPACE"
  else
    echo "cluster namespace $NAMESPACE already exists"
  fi
}

function importClusterID() {
  if [ -n "$RADOS_NAMESPACE" ]; then
    CLUSTER_ID_RBD=$(kubectl -n "$NAMESPACE" get cephblockpoolradosnamespace.ceph.rook.io/"$RADOS_NAMESPACE" -o jsonpath='{.status.info.clusterID}')
  fi
  if [ -n "$SUBVOLUME_GROUP" ]; then
    CLUSTER_ID_CEPHFS=$(kubectl -n "$NAMESPACE" get cephfilesystemsubvolumegroup.ceph.rook.io/"$SUBVOLUME_GROUP" -o jsonpath='{.status.info.clusterID}')
  fi
}

function importSecret() {
  if ! kubectl -n "$NAMESPACE" get secret "$MON_SECRET_NAME" &>/dev/null; then
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
  else
    echo "secret $MON_SECRET_NAME already exists"
  fi
}

function importConfigMap() {
  if ! kubectl -n "$NAMESPACE" get configmap "$MON_ENDPOINT_CONFIGMAP_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      configmap \
      "$MON_ENDPOINT_CONFIGMAP_NAME" \
      --from-literal=data="$ROOK_EXTERNAL_CEPH_MON_DATA" \
      --from-literal=mapping="$ROOK_EXTERNAL_MAPPING" \
      --from-literal=maxMonId="$ROOK_EXTERNAL_MAX_MON_ID"
  else
    echo "configmap $MON_ENDPOINT_CONFIGMAP_NAME already exists"
  fi
}

function importCsiRBDNodeSecret() {
  if ! kubectl -n "$NAMESPACE" get secret "rook-$CSI_RBD_NODE_SECRET_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      secret \
      generic \
      --type="kubernetes.io/rook" \
      "rook-""$CSI_RBD_NODE_SECRET_NAME" \
      --from-literal=userID="$CSI_RBD_NODE_SECRET_NAME" \
      --from-literal=userKey="$CSI_RBD_NODE_SECRET"
  else
    echo "secret rook-$CSI_RBD_NODE_SECRET_NAME already exists"
  fi
}

function importCsiRBDProvisionerSecret() {
  if ! kubectl -n "$NAMESPACE" get secret "rook-$CSI_RBD_PROVISIONER_SECRET_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      secret \
      generic \
      --type="kubernetes.io/rook" \
      "rook-""$CSI_RBD_PROVISIONER_SECRET_NAME" \
      --from-literal=userID="$CSI_RBD_PROVISIONER_SECRET_NAME" \
      --from-literal=userKey="$CSI_RBD_PROVISIONER_SECRET"
  else
    echo "secret $CSI_RBD_PROVISIONER_SECRET_NAME already exists"
  fi
}

function importCsiCephFSNodeSecret() {
  if ! kubectl -n "$NAMESPACE" get secret "rook-$CSI_CEPHFS_NODE_SECRET_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      secret \
      generic \
      --type="kubernetes.io/rook" \
      "rook-""$CSI_CEPHFS_NODE_SECRET_NAME" \
      --from-literal=adminID="$CSI_CEPHFS_NODE_SECRET_NAME" \
      --from-literal=adminKey="$CSI_CEPHFS_NODE_SECRET"
  else
    echo "secret $CSI_CEPHFS_NODE_SECRET_NAME already exists"
  fi
}

function importCsiCephFSProvisionerSecret() {
  if ! kubectl -n "$NAMESPACE" get secret "rook-$CSI_CEPHFS_PROVISIONER_SECRET_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      secret \
      generic \
      --type="kubernetes.io/rook" \
      "rook-""$CSI_CEPHFS_PROVISIONER_SECRET_NAME" \
      --from-literal=adminID="$CSI_CEPHFS_PROVISIONER_SECRET_NAME" \
      --from-literal=adminKey="$CSI_CEPHFS_PROVISIONER_SECRET"
  else
    echo "secret $CSI_CEPHFS_PROVISIONER_SECRET_NAME already exists"
  fi
}

function importRGWAdminOpsUser() {
  if ! kubectl -n "$NAMESPACE" get secret "$RGW_ADMIN_OPS_USER_SECRET_NAME" &>/dev/null; then
    kubectl -n "$NAMESPACE" \
      create \
      secret \
      generic \
      --type="kubernetes.io/rook" \
      "$RGW_ADMIN_OPS_USER_SECRET_NAME" \
      --from-literal=accessKey="$RGW_ADMIN_OPS_USER_ACCESS_KEY" \
      --from-literal=secretKey="$RGW_ADMIN_OPS_USER_SECRET_KEY"
  else
    echo "secret $RGW_ADMIN_OPS_USER_SECRET_NAME already exists"
  fi
}

function createECRBDStorageClass() {
  if ! kubectl -n "$NAMESPACE" get storageclass $RBD_STORAGE_CLASS_NAME &>/dev/null; then
    cat <<eof | kubectl create -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: $RBD_STORAGE_CLASS_NAME
provisioner: $RBD_PROVISIONER
parameters:
  clusterID: $CLUSTER_ID_RBD
  pool: $RBD_METADATA_EC_POOL_NAME
  dataPool: $RBD_POOL_NAME
  imageFormat: "2"
  imageFeatures: $ROOK_RBD_FEATURES
  csi.storage.k8s.io/provisioner-secret-name: "rook-$CSI_RBD_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/provisioner-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/controller-expand-secret-name:  "rook-$CSI_RBD_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/controller-expand-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/node-stage-secret-name: "rook-$CSI_RBD_NODE_SECRET_NAME"
  csi.storage.k8s.io/node-stage-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/fstype: ext4
allowVolumeExpansion: true
reclaimPolicy: Delete
eof
  else
    echo "storageclass $RBD_STORAGE_CLASS_NAME already exists"
  fi
}

function createRBDStorageClass() {
  if ! kubectl -n "$NAMESPACE" get storageclass $RBD_STORAGE_CLASS_NAME &>/dev/null; then
    cat <<eof | kubectl create -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: $RBD_STORAGE_CLASS_NAME
provisioner: $RBD_PROVISIONER
parameters:
  clusterID: $CLUSTER_ID_RBD
  pool: $RBD_POOL_NAME
  imageFormat: "2"
  imageFeatures: $ROOK_RBD_FEATURES
  csi.storage.k8s.io/provisioner-secret-name: "rook-$CSI_RBD_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/provisioner-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/controller-expand-secret-name:  "rook-$CSI_RBD_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/controller-expand-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/node-stage-secret-name: "rook-$CSI_RBD_NODE_SECRET_NAME"
  csi.storage.k8s.io/node-stage-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/fstype: ext4
allowVolumeExpansion: true
reclaimPolicy: Delete
eof
  else
    echo "storageclass $RBD_STORAGE_CLASS_NAME already exists"
  fi
}

function createCephFSStorageClass() {
  if ! kubectl -n "$NAMESPACE" get storageclass $CEPHFS_STORAGE_CLASS_NAME &>/dev/null; then
    cat <<eof | kubectl create -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: $CEPHFS_STORAGE_CLASS_NAME
provisioner: $CEPHFS_PROVISIONER
parameters:
  clusterID: $CLUSTER_ID_CEPHFS
  fsName: $CEPHFS_FS_NAME
  pool: $CEPHFS_POOL_NAME
  csi.storage.k8s.io/provisioner-secret-name: "rook-$CSI_CEPHFS_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/provisioner-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/controller-expand-secret-name: "rook-$CSI_CEPHFS_PROVISIONER_SECRET_NAME"
  csi.storage.k8s.io/controller-expand-secret-namespace: $NAMESPACE
  csi.storage.k8s.io/node-stage-secret-name: "rook-$CSI_CEPHFS_NODE_SECRET_NAME"
  csi.storage.k8s.io/node-stage-secret-namespace: $NAMESPACE
allowVolumeExpansion: true
reclaimPolicy: Delete
eof
  else
    echo "storageclass $CEPHFS_STORAGE_CLASS_NAME already exists"
  fi
}

########
# MAIN #
########
checkEnvVars
createClusterNamespace
importClusterID
importSecret
importConfigMap
if [ -n "$CSI_RBD_NODE_SECRET_NAME" ] && [ -n "$CSI_RBD_NODE_SECRET" ]; then
  importCsiRBDNodeSecret
fi
if [ -n "$CSI_RBD_PROVISIONER_SECRET_NAME" ] && [ -n "$CSI_RBD_PROVISIONER_SECRET" ]; then
  importCsiRBDProvisionerSecret
fi
if [ -n "$RGW_ADMIN_OPS_USER_ACCESS_KEY" ] && [ -n "$RGW_ADMIN_OPS_USER_SECRET_KEY" ]; then
  importRGWAdminOpsUser
fi
if [ -n "$CSI_CEPHFS_NODE_SECRET_NAME" ] && [ -n "$CSI_CEPHFS_NODE_SECRET" ]; then
  importCsiCephFSNodeSecret
fi
if [ -n "$CSI_CEPHFS_PROVISIONER_SECRET_NAME" ] && [ -n "$CSI_CEPHFS_PROVISIONER_SECRET" ]; then
  importCsiCephFSProvisionerSecret
fi
if [ -n "$RBD_POOL_NAME" ]; then
  if [ -n "$RBD_METADATA_EC_POOL_NAME" ]; then
    createECRBDStorageClass
  else
    createRBDStorageClass
  fi
fi
if [ -n "$CEPHFS_FS_NAME" ] && [ -n "$CEPHFS_POOL_NAME" ]; then
  createCephFSStorageClass
fi
