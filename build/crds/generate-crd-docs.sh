#! /usr/bin/env bash

SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
GEN_CRD_API_REFERENCE_DOC_VERSION=v0.3.0
SPECIFICATION_FILE=Documentation/CRDs/specification.md

install_generator() {
  go install github.com/ahmetb/gen-crd-api-reference-docs@${GEN_CRD_API_REFERENCE_DOC_VERSION}
}

run_gen() {
  if [[ $SKIP_GEN_CRD_DOCS = true ]]; then
    echo "SKIP_GEN_CRD_DOCS is set, skipping CRD docs generation"
  else
    echo "rebuilding specification.md"
    install_generator
    ${GOBIN}/gen-crd-api-reference-docs \
      -config="${SCRIPT_ROOT}/build/crds/crd-docs-config.json" \
      -template-dir="${SCRIPT_ROOT}/Documentation/gen-crd-api-reference-docs/template" \
      -api-dir="github.com/rook/rook/pkg/apis/ceph.rook.io" \
      -out-file="${SCRIPT_ROOT}/$SPECIFICATION_FILE"
  fi
}

run_gen
