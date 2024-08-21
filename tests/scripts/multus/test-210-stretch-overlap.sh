#!/usr/bin/env bash
set -xeuo pipefail

CONFFILE="stretch-overlap.yaml"
NAMESPACE="stretch-overlap"
OUTFILE="overlap-test.log"

kubectl create namespace "$NAMESPACE"

# create the expected serviceaccount in the namespace
# in Minikube/KinD, the tool needs no special permissions, so just the SA is fine
kubectl create serviceaccount rook-ceph-system --namespace "$NAMESPACE"

sed \
  -e "s|namespace:.*|namespace: $NAMESPACE|" \
  -e 's|publicNetwork:.*|publicNetwork: "default/public-net"|' \
  -e 's|clusterNetwork:.*|clusterNetwork: "default/cluster-net"|' \
  tests/scripts/multus/stretch.yaml >"$CONFFILE"
cat "$CONFFILE"

# Nodes do not yet have taints, which means worker and storage node type pods should overlap
# this is an important error condition to check because it ensures the test is valid
if ./rook --log-level DEBUG multus validation run --config "$CONFFILE" 2>&1 | tee "$OUTFILE"; then
  echo "Test was supposed to fail"
  exit 1
fi

# Check that node overlap error is present
grep 'RESULT: multus validation test failed: multus validation test failed: node types must not overlap: node type "worker-nodes" has overlap with node type "storage-nodes"' "$OUTFILE"
# should be resources left running for debugging
[[ -n "$(kubectl --namespace "$NAMESPACE" get pods --no-headers 2>/dev/null | grep -v Terminating)" ]]
