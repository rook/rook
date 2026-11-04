#!/usr/bin/env bash

set -x

# User parameters
: "${CLUSTER_NAMESPACE:="rook-ceph"}"
: "${OPERATOR_NAMESPACE:="$CLUSTER_NAMESPACE"}"
: "${ADDITIONAL_NAMESPACE:=""}"
: "${LOG_DIR:="test"}"

LOG_DIR="${LOG_DIR%/}" # remove trailing slash if necessary
mkdir -p "${LOG_DIR}"

CEPH_CMD="kubectl -n ${CLUSTER_NAMESPACE} exec deploy/rook-ceph-tools -- ceph --connect-timeout 10"

$CEPH_CMD -s >"${LOG_DIR}"/ceph-status.txt
$CEPH_CMD osd dump >"${LOG_DIR}"/ceph-osd-dump.txt
$CEPH_CMD report >"${LOG_DIR}"/ceph-report.txt
$CEPH_CMD auth ls >"${LOG_DIR}"/ceph-auth-ls.txt

# collects_secret_content reports whether a Secret's values are collected in full. Everything
# else is summarized, so a Secret appearing later under an unmatched name is withheld without
# anyone deciding again. The two prefix patterns are open rather than anchored on -keyring, so
# a new Secret named under them would be collected; every name they match today is a keyring.
#
# These are the keys a cephx rotation problem shows up in, and a cluster in that state may be
# unreachable by other means: `ceph auth ls` is answered by the mon, so the mon Secret in
# particular has to stay legible on its own. Within the cluster namespace they disclose nothing
# further, since ceph-auth-ls.txt holds the same entities' keys -- but CEPH_CMD is bound to
# CLUSTER_NAMESPACE, so a second cluster collected through ADDITIONAL_NAMESPACE, as the
# multi-cluster-mirroring and rgw-multisite canaries do, has no auth dump of its own.
function collects_secret_content() {
  case "$1" in
  rook-ceph-mon | rook-ceph-mgr-* | rook-ceph-mds-*) return 0 ;;
  *) return 1 ;;
  esac
}

# SECRET_SUMMARY_TEMPLATE describes a Secret without exposing it: one
# "secret <name> <type> <number of data keys>" record, then one "key <name> <size in bytes>"
# record per data key. The decoding happens inside kubectl's template engine, which provides
# base64decode, and only the decoded length is ever printed, so no part of a value leaves
# kubectl.
# shellcheck disable=SC2016 # $key and $value are go-template variables, not shell expansions
SECRET_SUMMARY_TEMPLATE='secret {{.metadata.name}} {{.type}} {{if .data}}{{len .data}}{{else}}0{{end}}{{"\n"}}'\
'{{if .data}}{{range $key, $value := .data}}'\
'key {{$key}} {{len (base64decode $value)}}{{"\n"}}'\
'{{end}}{{end}}'

# format_secret_summary admits a line only if it has the exact shape of one of those records:
# the right number of fields, a name of the kind Kubernetes allows, and a size that is a number.
# The shape is what is checked, not the leading word, because kubectl's output cannot be trusted
# line by line. A template it cannot execute makes it write the error, the template text and the
# object's raw JSON to this same stream, and any Secret field holding a newline makes whatever
# follows look like the start of a record; either can begin with the word 'secret' or 'key'
# while the rest of the line is something else entirely.
#
# Anything else is dropped, as is a run whose key records do not account for every data key, and
# a single warning takes its place -- a summary that could not be produced must not be
# indistinguishable from a Secret that holds nothing.
function format_secret_summary() {
  # The tests below expand their input, and this script runs under `set -x`, so without this
  # the rejected lines -- which is where a payload would be -- reach the job log, which is
  # where the warning then sends the reader. `local -` restores both options on return.
  local -
  set +x

  local name_re='^[-._a-zA-Z0-9]+$' type_re='^[-._a-zA-Z0-9/]+$' size_re='^[0-9]+$'
  local kind field1 field2 field3 extra
  local named=0 keys=0 expected=-1 rejected=0

  while read -r kind field1 field2 field3 extra; do
    if [[ -n "$extra" ]] || [[ ! "$field1" =~ $name_re ]]; then
      rejected=1
      continue
    fi
    case "$kind" in
    secret)
      if [[ "$named" -ne 0 ]] || [[ ! "$field2" =~ $type_re ]] || [[ ! "$field3" =~ $size_re ]]; then
        rejected=1
        continue
      fi
      named=1
      expected="$field3"
      printf 'name: %s\ntype: %s\ndata:\n' "$field1" "$field2"
      ;;
    key)
      if [[ "$named" -ne 1 ]] || [[ -n "$field3" ]] || [[ ! "$field2" =~ $size_re ]]; then
        rejected=1
        continue
      fi
      keys=$((keys + 1))
      printf '  %s: %s bytes\n' "$field1" "$field2"
      ;;
    *)
      rejected=1
      ;;
    esac
  done

  if [[ "$named" -ne 1 ]] || [[ "$keys" -ne "$expected" ]] || [[ "$rejected" -ne 0 ]]; then
    printf 'WARNING: this summary is incomplete; see the job log for what kubectl reported\n'
  fi
}

NAMESPACES=("$CLUSTER_NAMESPACE")
if [[ "$OPERATOR_NAMESPACE" != "$CLUSTER_NAMESPACE" ]]; then
  NAMESPACES+=("$OPERATOR_NAMESPACE")
fi

if [[ -n "${ADDITIONAL_NAMESPACE}" ]]; then
  NAMESPACES+=("${ADDITIONAL_NAMESPACE}")
fi

for NAMESPACE in "${NAMESPACES[@]}"; do
  # each namespace is a sub-directory for easier debugging
  NS_DIR="${LOG_DIR}"/namespace-"${NAMESPACE}"
  mkdir "${NS_DIR}"

  # describe every one of the k8s resources in the namespace which rook commonly uses
  for KIND in 'pod' 'deployment' 'job' 'daemonset' 'cm' 'pvc' 'sc'; do
    kubectl -n "$NAMESPACE" get "$KIND" -o wide >"${NS_DIR}"/"$KIND"-list.txt
    for resource in $(kubectl -n "$NAMESPACE" get "$KIND" -o jsonpath='{.items[*].metadata.name}'); do
      kubectl -n "$NAMESPACE" describe "$KIND" "$resource" >"${NS_DIR}"/"$KIND"-describe--"$resource".txt

      # collect logs for pods along the way
      if [[ "$KIND" == 'pod' ]]; then
        kubectl -n "$NAMESPACE" logs --all-containers "$resource" >"${NS_DIR}"/logs--"$resource".txt
      fi
    done
  done

  # Secrets get a block of their own because no other kind needs this treatment. This directory
  # is published as a workflow artifact, so everything written here is readable by anyone who
  # can read the artifact, and base64 is encoding rather than protection. Most Secrets are
  # therefore summarized instead of dumped: what these logs are read for is whether a Secret
  # exists and carries the keys it is supposed to, which the summary answers. `kubectl describe
  # secret` is not an alternative, since it prints the token of a service-account-token Secret
  # in full.
  for secret in $(kubectl -n "$NAMESPACE" get secrets -o jsonpath='{.items[*].metadata.name}'); do
    if collects_secret_content "$secret"; then
      kubectl -n "$NAMESPACE" get -o yaml secret "$secret" >"${NS_DIR}"/secret-get--"$secret".txt
    else
      kubectl -n "$NAMESPACE" get secret "$secret" -o go-template="$SECRET_SUMMARY_TEMPLATE" |
        format_secret_summary >"${NS_DIR}"/secret-get--"$secret".txt
    fi
  done

  # describe every one of the custom resources in the namespace since all should be rook-related and
  # they aren't captured by 'kubectl get all'
  for CRD in $(kubectl get crds -o jsonpath='{.items[*].metadata.name}'); do
    for resource in $(kubectl -n "$NAMESPACE" get "$CRD" -o jsonpath='{.items[*].metadata.name}'); do
      crd_main_type="${CRD%%.*}" # e.g., for cephclusters.ceph.rook.io, only use 'cephclusters'
      kubectl -n "$NAMESPACE" describe "$CRD" "$resource" >>"${NS_DIR}"/"$crd_main_type"-describe--"$resource".txt
    done
  done

  # do simple 'get all' calls for resources we don't often want to look at
  kubectl get all -n "$NAMESPACE" -o wide >"${NS_DIR}"/all-wide.txt
  kubectl get all -n "$NAMESPACE" -o yaml >"${NS_DIR}"/all-yaml.txt
done

sudo lsblk | sudo tee -a "${LOG_DIR}"/lsblk.txt
journalctl -o short-precise --dmesg >"${LOG_DIR}"/dmesg.txt
journalctl >"${LOG_DIR}"/journalctl.txt
