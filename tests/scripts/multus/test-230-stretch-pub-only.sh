#!/usr/bin/env bash
set -xeuo pipefail

CONFFILE="stretch-pub-only.yaml"
NAMESPACE="stretch-pub-only"
OUTFILE="stretch-pub-test.log"

kubectl create namespace "$NAMESPACE"

# create the expected serviceaccount in the namespace
# in Minikube/KinD, the tool needs no special permissions, so just the SA is fine
kubectl create serviceaccount rook-ceph-system --namespace "$NAMESPACE"

# all daemons get public network, so all should get started

sed \
  -e "s|namespace:.*|namespace: $NAMESPACE|" \
  -e 's|publicNetwork:.*|publicNetwork: "default/public-net"|' \
  -e 's|clusterNetwork:.*|clusterNetwork: ""|' \
  tests/scripts/multus/stretch.yaml >"$CONFFILE"
cat "$CONFFILE"

./rook --log-level DEBUG multus validation run --config "$CONFFILE" 2>&1 | tee "$OUTFILE"

# 3 node types:
# arbiter-node (1 node, on control-plane)
grep 'starting 0 osd validation clients for node type "arbiter-node"' "$OUTFILE"
grep 'starting 1 other (non-OSD) validation clients for node type "arbiter-node"' "$OUTFILE"

# storage-nodes (2 nodes)
grep 'starting 2 osd validation clients for node type "storage-nodes"' "$OUTFILE"
grep 'starting 3 other (non-OSD) validation clients for node type "storage-nodes"' "$OUTFILE"

# worker-nodes (1 node)
grep 'starting 0 osd validation clients for node type "worker-nodes"' "$OUTFILE"
grep 'starting 2 other (non-OSD) validation clients for node type "worker-nodes"' "$OUTFILE"

# total
grep "all 13 clients are 'Ready'" "$OUTFILE"

# should be no non-terminating resources in namespace after successful test
[[ -z "$(kubectl --namespace "$NAMESPACE" get pods --no-headers 2>/dev/null | grep -v Terminating)" ]]
