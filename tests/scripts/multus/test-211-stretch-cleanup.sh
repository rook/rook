#!/usr/bin/env bash
set -xeuo pipefail

NAMESPACE="stretch-overlap"

./rook --log-level DEBUG multus validation cleanup --namespace "$NAMESPACE"

# should be no non-terminating resources in namespace
[[ -z "$(kubectl --namespace "$NAMESPACE" get pods --no-headers 2>/dev/null | grep -v Terminating)" ]]
