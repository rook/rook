#!/usr/bin/env bash

alias ka="kubectl -n rook-a"
alias kb="kubectl -n rook-b"

# Assume cluster.yaml and operator.yaml have already been copied with their customizations

echo "Generating files for cluster a"
cp ../csi-operator.yaml csi-operator-a.yaml
cp ../common.yaml common-a.yaml
cp ../object-test.yaml object-a.yaml
cp ../filesystem-test.yaml filesystem-a.yaml
export ROOK_OPERATOR_NAMESPACE="rook-a"
export ROOK_CLUSTER_NAMESPACE="rook-a"
sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:operator/\1:$ROOK_OPERATOR_NAMESPACE:\2 # serviceaccount:namespace:operator/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:cluster/\1:$ROOK_CLUSTER_NAMESPACE:\2 # serviceaccount:namespace:cluster/g" \
    -e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # driver:namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE.\2 # driver:namespace:cluster/g" \
common-a.yaml csi-operator-a.yaml operator-a.yaml cluster-a.yaml object-a.yaml filesystem-a.yaml # add other files or change these as desired for your config

echo "Generating files for cluster b"
cp ../common.yaml common-b.yaml
cp ../object-test.yaml object-b.yaml
cp ../filesystem-test.yaml filesystem-b.yaml
export ROOK_OPERATOR_NAMESPACE="rook-b"
export ROOK_CLUSTER_NAMESPACE="rook-b"
sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:operator/\1:$ROOK_OPERATOR_NAMESPACE:\2 # serviceaccount:namespace:operator/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:cluster/\1:$ROOK_CLUSTER_NAMESPACE:\2 # serviceaccount:namespace:cluster/g" \
    -e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # driver:namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE.\2 # driver:namespace:cluster/g" \
common-b.yaml operator-b.yaml cluster-b.yaml object-b.yaml filesystem-b.yaml # add other files or change these as desired for your config

echo "Cleaning up temp files"
rm *.bak

echo "Done"

# Apply the manifests

#kubectl apply -f ../crds.yaml

#kubectl apply -f common-a.yaml -f operator-a.yaml -f cluster-a.yaml -f object-a.yaml -f filesystem-a.yaml # add other files as desired for yourconfig

#kubectl apply -f common-b.yaml -f operator-b.yaml -f cluster-b.yaml -f object-b.yaml -f filesystem-b.yaml # add other files as desired for yourconfig


# EXTRA STEPS

# common.yaml
# 1. Remove all CSI-related resources since we only require CSI operator

# Manifests for rook-b
# 1. cluster-a.yaml:
#   - Change dataDirHostPath
#   - Change deviceFilter for the minikube OSD

# Manifests for rook-b
# 1. common-b.yaml:
#  - Remove all resources of type ClusterRole
# 2. csi-operator-b.yaml:
#  - Remove CRDs
#  - Remove ClusterRoles
#  - Remove CSI operator deployment and service
#  - Change namespace to rook-b
# 3. cluster-b.yaml:
#   - Change dataDirHostPath
