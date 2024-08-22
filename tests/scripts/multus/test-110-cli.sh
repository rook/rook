#!/usr/bin/env bash
set -xeuo pipefail

NAMESPACE="cli-test"
OUTFILE="cli-test.log"

kubectl create namespace "$NAMESPACE"

# create the expected serviceaccount in the namespace
# in Minikube/KinD, the tool needs no special permissions, so just the SA is fine
kubectl create serviceaccount rook-ceph-system --namespace "$NAMESPACE"

./rook --log-level DEBUG multus validation run \
  --namespace "$NAMESPACE" \
  --public-network default/public-net \
  --cluster-network default/cluster-net \
  --daemons-per-node 2 \
  2>&1 | tee "$OUTFILE"

grep "starting 2 osd validation clients for node type" "$OUTFILE"
grep "starting 0 other (non-OSD) validation clients for node type" "$OUTFILE"
grep "all 6 clients are 'Ready'" "$OUTFILE"

# should be no non-terminating resources in namespace after successful test
[[ -z "$(kubectl --namespace "$NAMESPACE" get pods --no-headers 2>/dev/null | grep -v Terminating)" ]]
