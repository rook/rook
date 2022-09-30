#!/bin/bash

set -ex

VERSION='3.0.1'

print_error() {
  >&2 echo -e "$@"
}

fail() {
  local code=${2:-1}
  [[ -n $1 ]] && print_error "$1"
  # shellcheck disable=SC2086
  exit $code
}

has_cmd() {
  local command=${1?command is required}
  command -v "$command" > /dev/null 2>&1
}

require_cmds() {
  local cmds=("${@?at least one command is required}")
  local errors=()

  # accumulate a list of all missing commands before failing to reduce end-user
  # install/retry cycles
  for c in "${cmds[@]}"; do
    if ! has_cmd "$c"; then
      errors+=("prog: ${c} is required")
    fi
  done

  if [[ ${#errors[@]} -ne 0 ]]; then
    for e in "${errors[@]}"; do
      print_error "$e"
    done

    fail 'failed because of missing commands'
  fi
}

waitfor() {
  xtrace=$(set +o|grep xtrace); set +x
  local ns=${1?namespace is required}; shift
  local type=${1?type is required}; shift

  echo "Waiting for $type $*"
  # wait for resource to exist. See: https://github.com/kubernetes/kubernetes/issues/83242
  until kubectl -n "$ns" get "$type" "$@" -o=jsonpath='{.items[0].metadata.name}' >/dev/null 2>&1; do
    echo "Waiting for $type $*"
    sleep 1
  done
  eval "$xtrace"
}

waitforpod() {
  xtrace=$(set +o|grep xtrace); set +x
  local ns=${1?namespace is required}; shift

  # wait for pod to exist
  waitfor "$ns" pod "$@"

  # wait for pod to be ready
  kubectl -n "$ns" wait --for=condition=ready --timeout=180s pod "$@"
  eval "$xtrace"
}

require_cmds helm kubectl ldapsearch

helm repo add helm-openldap https://jp-gouin.github.io/helm-openldap/
helm repo update

helm upgrade --install \
  --atomic \
  openldap helm-openldap/openldap-stack-ha \
  --create-namespace --namespace openldap \
  --version "v${VERSION}" \
  --values - <<EOF
---
persistence:
  enabled: false
phpldapadmin:
  enabled: false
ltb-passwd:
  enabled: false
replication:
  enabled: false
replicaCount: 1

customLdifFiles:
  01-foo-user.ldif: |-
    dn: uid=foo,dc=example,dc=org
    uid: foo
    objectClass: top
    objectClass: person
    objectClass: posixaccount
    cn: foo
    sn: bar
    homeDirectory: /home/foo
    uidNumber: 70054
    gidNumber: 70054
    userPassword: baz
EOF

waitforpod openldap openldap-0

LDAP_SVC=$(kubectl -n openldap get svc openldap -o jsonpath="{.spec.clusterIP}")

timeout 60 bash <<EOF
until ldapsearch -x -o nettimeout=5 -H "ldap://${LDAP_SVC}:389" -D "cn=admin,dc=example,dc=org" -w Not@SecurePassw0rd -b dc=example,dc=org '(uid=foo)'; do
  echo "waiting for openldap to answer queries"
  sleep 5
done
EOF
