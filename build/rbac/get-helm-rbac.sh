#!/usr/bin/env bash
set -eEuox pipefail

: ${HELM:=helm}

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
pushd "$SCRIPT_DIR"

${HELM} template ../../deploy/charts/rook-ceph \
                  --namespace rook-ceph \
                  --set crds.enabled=false | ./keep-rbac-yaml.sh > rbac.yaml

popd
