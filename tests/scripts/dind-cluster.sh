#!/bin/bash
# Copyright 2017 Mirantis
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

if [ $(uname) = Darwin ]; then
  readlinkf(){ perl -MCwd -e 'print Cwd::abs_path shift' "$1";}
else
  readlinkf(){ readlink -f "$1"; }
fi
DIND_ROOT="$(cd $(dirname "$(readlinkf "${BASH_SOURCE}")"); pwd)"

RUN_ON_BTRFS_ANYWAY="${RUN_ON_BTRFS_ANYWAY:-}"
if [[ ! ${RUN_ON_BTRFS_ANYWAY} ]] && docker info| grep -q '^Storage Driver: btrfs'; then
  echo "ERROR: Docker is using btrfs storage driver which is unsupported by kubeadm-dind-cluster" >&2
  echo "Please refer to the documentation for more info." >&2
  echo "Set RUN_ON_BTRFS_ANYWAY to non-empty string to continue anyway." >&2
  exit 1
fi

# In case of moby linux, -v will not work so we can't
# mount /lib/modules and /boot
is_moby_linux=
if docker info|grep -s '^Kernel Version: .*-moby$' > /dev/null 2>&1; then
    is_moby_linux=1
fi

#%CONFIG%

if [[ ! ${EMBEDDED_CONFIG:-} ]]; then
  source "${DIND_ROOT}/config.sh"
fi

CNI_PLUGIN="${CNI_PLUGIN:-bridge}"
DIND_SUBNET="${DIND_SUBNET:-10.192.0.0}"
dind_ip_base="$(echo "${DIND_SUBNET}" | sed 's/\.0$//')"
DIND_IMAGE="${DIND_IMAGE:-}"
BUILD_KUBEADM="${BUILD_KUBEADM:-}"
BUILD_HYPERKUBE="${BUILD_HYPERKUBE:-}"
APISERVER_PORT=${APISERVER_PORT:-8080}
NUM_NODES=${NUM_NODES:-2}
LOCAL_KUBECTL_VERSION=${LOCAL_KUBECTL_VERSION:-}
KUBECTL_DIR="${KUBECTL_DIR:-${HOME}/.kubeadm-dind-cluster}"
DASHBOARD_URL="${DASHBOARD_URL:-https://rawgit.com/kubernetes/dashboard/bfab10151f012d1acc5dfb1979f3172e2400aa3c/src/deploy/kubernetes-dashboard.yaml}"
SKIP_SNAPSHOT="${SKIP_SNAPSHOT:-}"
E2E_REPORT_DIR="${E2E_REPORT_DIR:-}"

if [[ ! ${LOCAL_KUBECTL_VERSION:-} && ${DIND_IMAGE:-} =~ :(v[0-9]+\.[0-9]+)$ ]]; then
  LOCAL_KUBECTL_VERSION="${BASH_REMATCH[1]}"
fi

function dind::need-source {
  if [[ ! -f cluster/kubectl.sh ]]; then
    echo "$0 must be called from the Kubernetes repository root directory" 1>&2
    exit 1
  fi
}

build_tools_dir="build"
use_k8s_source=y
if [[ ! ${BUILD_KUBEADM} && ! ${BUILD_HYPERKUBE} ]]; then
  use_k8s_source=
fi
if [[ ${use_k8s_source} ]]; then
  dind::need-source
  kubectl=cluster/kubectl.sh
  if [[ ! -f ${build_tools_dir}/common.sh ]]; then
    build_tools_dir="build-tools"
  fi
else
  if [[ ! ${LOCAL_KUBECTL_VERSION:-} ]] && ! hash kubectl 2>/dev/null; then
    echo "You need kubectl binary in your PATH to use prebuilt DIND image" 1>&2
    exit 1
  fi
  kubectl=kubectl
fi

busybox_image="busybox:1.26.2"
e2e_base_image="golang:1.7.1"
sys_volume_args=()
build_volume_args=()

function dind::set-build-volume-args {
  if [ ${#build_volume_args[@]} -gt 0 ]; then
    return 0
  fi
  build_container_name=
  if [ -n "${KUBEADM_DIND_LOCAL:-}" ]; then
    build_volume_args=(-v "$PWD:/go/src/k8s.io/kubernetes")
  else
    build_container_name="$(KUBE_ROOT=$PWD &&
                            . ${build_tools_dir}/common.sh &&
                            kube::build::verify_prereqs >&2 &&
                            echo "${KUBE_DATA_CONTAINER_NAME:-${KUBE_BUILD_DATA_CONTAINER_NAME}}")"
    build_volume_args=(--volumes-from "${build_container_name}")
  fi
}

function dind::volume-exists {
  local name="$1"
  if docker volume inspect "${name}" >& /dev/null; then
    return 0
  fi
  return 1
}

function dind::create-volume {
  local name="$1"
  docker volume create --label mirantis.kubeadm_dind_cluster --name "${name}" >/dev/null
}

# We mount /boot and /lib/modules into the container
# below to in case some of the workloads need them.
# This includes virtlet, for instance. Also this may be
# useful in future if we want DIND nodes to pass
# preflight checks.
# Unfortunately we can't do this when using Mac Docker
# (unless a remote docker daemon on Linux is used)
# NB: there's no /boot on recent Mac dockers
function dind::prepare-sys-mounts {
  if [[ ! ${is_moby_linux} ]]; then
    sys_volume_args=()
    if [[ -d /boot ]]; then
      sys_volume_args+=(-v /boot:/boot)
    fi
    if [[ -d /lib/modules ]]; then
      sys_volume_args+=(-v /lib/modules:/lib/modules)
    fi
    return 0
  fi
  if ! dind::volume-exists kubeadm-dind-sys; then
    dind::step "Saving a copy of docker host's /lib/modules"
    dind::create-volume kubeadm-dind-sys
    # Use a dirty nsenter trick to fool Docker on Mac and grab system
    # /lib/modules into sys.tar file on kubeadm-dind-sys volume.
    local nsenter="nsenter --mount=/proc/1/ns/mnt --"
    docker run \
           --rm \
           --privileged \
           -v kubeadm-dind-sys:/dest \
           --pid=host \
           "${busybox_image}" \
           /bin/sh -c \
           "if ${nsenter} test -d /lib/modules; then ${nsenter} tar -C / -c lib/modules >/dest/sys.tar; fi"
  fi
  sys_volume_args=(-v kubeadm-dind-sys:/dind-sys)
}

tmp_containers=()

function dind::cleanup {
  if [ ${#tmp_containers[@]} -gt 0 ]; then
    for name in "${tmp_containers[@]}"; do
      docker rm -vf "${name}" 2>/dev/null
    done
  fi
}

trap dind::cleanup EXIT

function dind::check-image {
  local name="$1"
  if docker inspect --format 'x' "${name}" >&/dev/null; then
    return 0
  else
    return 1
  fi
}

function dind::filter-make-output {
  # these messages make output too long and make Travis CI choke
  egrep -v --line-buffered 'I[0-9][0-9][0-9][0-9] .*(parse|conversion|defaulter|deepcopy)\.go:[0-9]+\]'
}

function dind::run-build-command {
    # this is like build/run.sh, but it doesn't rsync back the binaries,
    # only the generated files.
    local cmd=("$@")
    (
        # The following is taken from build/run.sh and build/common.sh
        # of Kubernetes source tree. It differs in
        # --filter='+ /_output/dockerized/bin/**'
        # being removed from rsync
        . ${build_tools_dir}/common.sh
        kube::build::verify_prereqs
        kube::build::build_image
        kube::build::run_build_command "$@"

        kube::log::status "Syncing out of container"

        kube::build::start_rsyncd_container

        local rsync_extra=""
        if (( ${KUBE_VERBOSE} >= 6 )); then
            rsync_extra="-iv"
        fi

        # The filter syntax for rsync is a little obscure. It filters on files and
        # directories.  If you don't go in to a directory you won't find any files
        # there.  Rules are evaluated in order.  The last two rules are a little
        # magic. '+ */' says to go in to every directory and '- /**' says to ignore
        # any file or directory that isn't already specifically allowed.
        #
        # We are looking to copy out all of the built binaries along with various
        # generated files.
        kube::build::rsync \
            --filter='- /vendor/' \
            --filter='- /_temp/' \
            --filter='+ zz_generated.*' \
            --filter='+ generated.proto' \
            --filter='+ *.pb.go' \
            --filter='+ types.go' \
            --filter='+ */' \
            --filter='- /**' \
            "rsync://k8s@${KUBE_RSYNC_ADDR}/k8s/" "${KUBE_ROOT}"

        kube::build::stop_rsyncd_container
    )
}

function dind::make-for-linux {
  local copy="$1"
  shift
  dind::step "Building binaries:" "$*"
  if [ -n "${KUBEADM_DIND_LOCAL:-}" ]; then
    dind::step "+ make WHAT=\"$*\""
    make WHAT="$*" 2>&1 | dind::filter-make-output
  elif [ "${copy}" = "y" ]; then
    dind::step "+ ${build_tools_dir}/run.sh make WHAT=\"$*\""
    "${build_tools_dir}/run.sh" make WHAT="$*" 2>&1 | dind::filter-make-output
  else
    dind::step "+ [using the build container] make WHAT=\"$*\""
    dind::run-build-command make WHAT="$*" 2>&1 | dind::filter-make-output
  fi
}

function dind::check-binary {
  local filename="$1"
  local dockerized="_output/dockerized/bin/linux/amd64/${filename}"
  local plain="_output/local/bin/linux/amd64/${filename}"
  dind::set-build-volume-args
  # FIXME: don't hardcode amd64 arch
  if [ -n "${KUBEADM_DIND_LOCAL:-${force_local:-}}" ]; then
    if [ -f "${dockerized}" -o -f "${plain}" ]; then
      return 0
    fi
  elif docker run --rm "${build_volume_args[@]}" \
              "${busybox_image}" \
              test -f "/go/src/k8s.io/kubernetes/${dockerized}" >&/dev/null; then
    return 0
  fi
  return 1
}

function dind::ensure-downloaded-kubectl {
  local kubectl_url
  local kubectl_sha1
  local kubectl_sha1_linux
  local kubectl_sha1_darwin
  local kubectl_link
  local kubectl_os
  local full_kubectl_version

  case "${LOCAL_KUBECTL_VERSION}" in
     v1.5)
      full_kubectl_version=v1.5.4
      kubectl_sha1_linux=15d8430dc52b1f3772b88bc6a236c8fa58e07c0d
      kubectl_sha1_darwin=5e671ba792567574eea48be4eddd844ba2f07c27
      ;;
    v1.6)
      full_kubectl_version=v1.6.6
      kubectl_sha1_linux=41153558717f3206d37f5bf34232a303ae4dade1
      kubectl_sha1_darwin=9795098e7340764b96a83e50676886d29e792033
      ;;
    v1.7)
      full_kubectl_version=v1.7.0
      kubectl_sha1_linux=c92ec52c02ec10a1ab54132d3cc99ad6f68c530e
      kubectl_sha1_darwin=2e2708b873accafb1be8f328008e3d41a6a32c08
      ;;
    "")
      return 0
      ;;
    *)
      echo "Invalid kubectl version" >&2
      exit 1
  esac

  export PATH="${KUBECTL_DIR}:$PATH"

  if [ $(uname) = Darwin ]; then
    kubectl_sha1="${kubectl_sha1_darwin}"
    kubectl_os=darwin
  else
    kubectl_sha1="${kubectl_sha1_linux}"
    kubectl_os=linux
  fi
  local link_target="kubectl-${full_kubectl_version}"
  local link_name="${KUBECTL_DIR}"/kubectl
  if [[ -h "${link_name}" && "$(readlink "${link_name}")" = "${link_target}" ]]; then
    return 0
  fi

  local path="${KUBECTL_DIR}/${link_target}"
  if [[ ! -f "${path}" ]]; then
    mkdir -p "${KUBECTL_DIR}"
    curl -sSLo "${path}" "https://storage.googleapis.com/kubernetes-release/release/${full_kubectl_version}/bin/${kubectl_os}/amd64/kubectl"
    echo "${kubectl_sha1} ${path}" | sha1sum -c
    chmod +x "${path}"
  fi

  ln -fs "${link_target}" "${KUBECTL_DIR}/kubectl"
}

function dind::ensure-kubectl {
  if [[ ! ${use_k8s_source} ]]; then
    # already checked on startup
    dind::ensure-downloaded-kubectl
    return 0
  fi
  if [ $(uname) = Darwin ]; then
    if [ ! -f _output/local/bin/darwin/amd64/kubectl ]; then
      dind::step "Building kubectl"
      dind::step "+ make WHAT=cmd/kubectl"
      make WHAT=cmd/kubectl 2>&1 | dind::filter-make-output
    fi
  elif ! force_local=y dind::check-binary kubectl; then
    dind::make-for-linux y cmd/kubectl
  fi
}

function dind::ensure-binaries {
  local -a to_build=()
  for name in "$@"; do
    if ! dind::check-binary "$(basename "${name}")"; then
      to_build+=("${name}")
    fi
  done
  if [ "${#to_build[@]}" -gt 0 ]; then
    dind::make-for-linux n "${to_build[@]}"
  fi
  return 0
}

function dind::ensure-network {
  if ! docker network inspect kubeadm-dind-net >&/dev/null; then
    docker network create --subnet="${DIND_SUBNET}/16" kubeadm-dind-net >/dev/null
  fi
}

function dind::ensure-volume {
  local reuse_volume=
  if [[ $1 = -r ]]; then
    reuse_volume=1
    shift
  fi
  local name="$1"
  if dind::volume-exists "${name}"; then
    if [[ ! {reuse_volume} ]]; then
      docker volume rm "${name}" >/dev/null
    fi
  elif [[ ${reuse_volume} ]]; then
    echo "*** Failed to locate volume: ${name}" 1>&2
    return 1
  fi
  dind::create-volume "${name}"
}

function dind::run {
  local reuse_volume=
  if [[ $1 = -r ]]; then
    reuse_volume="-r"
    shift
  fi
  local container_name="${1:-}"
  local ip="${2:-}"
  local netshift="${3:-}"
  local portforward="${4:-}"
  if [[ $# -gt 4 ]]; then
    shift 4
  else
    shift $#
  fi
  local -a opts=(--ip "${ip}" "$@")
  local -a args=("systemd.setenv=CNI_PLUGIN=${CNI_PLUGIN}")

  if [[ ! "${container_name}" ]]; then
    echo >&2 "Must specify container name"
    exit 1
  fi

  # remove any previously created containers with the same name
  docker rm -vf "${container_name}" >&/dev/null || true

  if [[ "$portforward" ]]; then
    opts+=(-p "$portforward")
  fi

  if [[ ${CNI_PLUGIN} = bridge && ${netshift} ]]; then
    args+=("systemd.setenv=CNI_BRIDGE_NETWORK_OFFSET=0.0.${netshift}.0")
  fi

  opts+=(${sys_volume_args[@]+"${sys_volume_args[@]}"})

  dind::step "Starting DIND container:" "${container_name}"

  if [[ ! ${is_moby_linux} ]]; then
    opts+=(-v /boot:/boot -v /lib/modules:/lib/modules)
  fi

  volume_name="kubeadm-dind-${container_name}"
  dind::ensure-network
  dind::ensure-volume ${reuse_volume} "${volume_name}"

  # TODO: create named volume for binaries and mount it to /k8s
  # in case of the source build

  # Start the new container.
  docker run \
         -d --privileged \
         --net kubeadm-dind-net \
         --name "${container_name}" \
         --hostname "${container_name}" \
         -l mirantis.kubeadm_dind_cluster \
         -v ${volume_name}:/dind \
         -v /lib/modules:/lib/modules \
         -v /sbin/modprobe:/sbin/modprobe \
         -v /dev:/dev \
         -v /sys/bus:/sys/bus \
         -v /var/run/docker.sock:/opt/outer-docker.sock \
         ${opts[@]+"${opts[@]}"} \
         "${DIND_IMAGE}" \
         ${args[@]+"${args[@]}"}
}

function dind::kubeadm {
  local container_id="$1"
  shift
  dind::step "Running kubeadm:" "$*"
  status=0
  # See image/bare/wrapkubeadm.
  # Capturing output is necessary to grab flags for 'kubeadm join'
  if ! docker exec "${container_id}" wrapkubeadm "$@" 2>&1 | tee /dev/fd/2; then
    echo "*** kubeadm failed" >&2
    return 1
  fi
  return ${status}
}

# function dind::bare {
#   local container_name="${1:-}"
#   if [[ ! "${container_name}" ]]; then
#     echo >&2 "Must specify container name"
#     exit 1
#   fi
#   shift
#   run_opts=(${@+"$@"})
#   dind::run "${container_name}"
# }

function dind::configure-kubectl {
  dind::step "Setting cluster config"
  "${kubectl}" config set-cluster dind --server="http://localhost:${APISERVER_PORT}" --insecure-skip-tls-verify=true
  "${kubectl}" config set-context dind --cluster=dind
  "${kubectl}" config use-context dind
}

force_make_binaries=
function dind::set-master-opts {
  master_opts=()
  if [[ ${BUILD_KUBEADM} || ${BUILD_HYPERKUBE} ]]; then
    # share binaries pulled from the build container between nodes
    dind::ensure-volume "dind-k8s-binaries"
    dind::set-build-volume-args
    master_opts+=("${build_volume_args[@]}" -v dind-k8s-binaries:/k8s)
    local -a bins
    if [[ ${BUILD_KUBEADM} ]]; then
      master_opts+=(-e KUBEADM_SOURCE=build://)
      bins+=(cmd/kubeadm)
    fi
    if [[ ${BUILD_HYPERKUBE} ]]; then
      master_opts+=(-e HYPERKUBE_SOURCE=build://)
      bins+=(cmd/hyperkube)
    fi
    if [[ ${force_make_binaries} ]]; then
      dind::make-for-linux n "${bins[@]}"
    else
      dind::ensure-binaries "${bins[@]}"
    fi
  fi
}

cached_use_rbac=
function dind::use-rbac {
  # we use rbac in case of k8s 1.6
  if [[ ${cached_use_rbac} ]]; then
    [[ ${cached_use_rbac} = 1 ]] && return 0 || return 1
  fi
  cached_use_rbac=0
  if "${kubectl}" version --short >& /dev/null && ! "${kubectl}" version --short | grep -q 'Server Version: v1\.5\.'; then
    cached_use_rbac=1
    return 0
  fi
  return 1
}

function dind::deploy-dashboard {
  dind::step "Deploying k8s dashboard"
  "${kubectl}" create -f "${DASHBOARD_URL}"
  if dind::use-rbac; then
    # https://kubernetes-io-vnext-staging.netlify.com/docs/admin/authorization/rbac/#service-account-permissions
    # Thanks @liggitt for the hint
    "${kubectl}" create clusterrolebinding add-on-cluster-admin --clusterrole=cluster-admin --serviceaccount=kube-system:default
  fi
}

function dind::init {
  local -a opts
  dind::set-master-opts
  local master_ip="${dind_ip_base}.2"
  local container_id=$(dind::run kube-master "${master_ip}" 1 127.0.0.1:${APISERVER_PORT}:8080 ${master_opts[@]+"${master_opts[@]}"})
  # FIXME: I tried using custom tokens with 'kubeadm ex token create' but join failed with:
  # 'failed to parse response as JWS object [square/go-jose: compact JWS format must have three parts]'
  # So we just pick the line from 'kubeadm init' output
  local kube_version_flag=""
  if [[ ${BUILD_KUBEADM} ]]; then
    # FIXME: this is temporary fix for kubeadm trying to get a non-existent release URL.
    # It doesn't change the fact that we're deploying custom-built k8s version
    kube_version_flag="--kubernetes-version=stable-1.6"
  fi
  kubeadm_join_flags="$(dind::kubeadm "${container_id}" init --pod-network-cidr=10.244.0.0/16 --skip-preflight-checks ${kube_version_flag} "$@" | grep '^ *kubeadm join' | sed 's/^ *kubeadm join //')"
  dind::configure-kubectl
  dind::deploy-dashboard
}

function dind::create-node-container {
  local reuse_volume=
  if [[ $1 = -r ]]; then
    reuse_volume="-r"
    shift
  fi
  # if there's just one node currently, it's master, thus we need to use
  # kube-node-1 hostname, if there are two nodes, we should pick
  # kube-node-2 and so on
  local next_node_index=${1:-$(docker ps -q --filter=label=mirantis.kubeadm_dind_cluster | wc -l | sed 's/^ *//g')}
  local node_ip="${dind_ip_base}.$((next_node_index + 2))"
  local -a opts
  if [[ ${BUILD_KUBEADM} || ${BUILD_HYPERKUBE} ]]; then
    opts+=(-v dind-k8s-binaries:/k8s)
    if [[ ${BUILD_KUBEADM} ]]; then
      opts+=(-e KUBEADM_SOURCE=build://)
    fi
    if [[ ${BUILD_HYPERKUBE} ]]; then
      opts+=(-e HYPERKUBE_SOURCE=build://)
    fi
  fi
  dind::run ${reuse_volume} kube-node-${next_node_index} ${node_ip} $((next_node_index + 1)) "" ${opts[@]+"${opts[@]}"}
}

function dind::join {
  local container_id="$1"
  shift
  dind::kubeadm "${container_id}" join --skip-preflight-checks "$@" >/dev/null
}

function dind::escape-e2e-name {
    sed 's/[]\$*.^|()[]/\\&/g; s/\s\+/\\s+/g' <<< "$1" | tr -d '\n'
}

function dind::accelerate-kube-dns {
  dind::step "Patching kube-dns deployment to make it start faster"
  # Could do this on the host, too, but we don't want to require jq here
  # TODO: do this in wrapkubeadm
  # 'kubectl version --short' is a quick check for kubectl 1.4
  # which doesn't support 'kubectl apply --force'
  docker exec kube-master /bin/bash -c \
         "kubectl get deployment kube-dns -n kube-system -o json | jq '.spec.template.spec.containers[0].readinessProbe.initialDelaySeconds = 3|.spec.template.spec.containers[0].readinessProbe.periodSeconds = 3' | if kubectl version --short >&/dev/null; then kubectl apply --force -f -; else kubectl apply -f -; fi"
}

function dind::component-ready {
  local label="$1"
  local out
  if ! out="$("${kubectl}" get pod -l "${label}" -n kube-system \
                           -o jsonpath='{ .items[*].status.conditions[?(@.type == "Ready")].status }' 2>/dev/null)"; then
    return 1
  fi
  if ! grep -v False <<<"${out}" | grep -q True; then
    return 1
  fi
  return 0
}

function dind::kill-failed-pods {
  local pods
  # workaround for https://github.com/kubernetes/kubernetes/issues/36482
  if ! pods="$(kubectl get pod -n kube-system -o jsonpath='{ .items[?(@.status.phase == "Failed")].metadata.name }' 2>/dev/null)"; then
    return
  fi
  for name in ${pods}; do
    kubectl delete pod --now -n kube-system "${name}" >&/dev/null || true
  done
}

function dind::wait-for-ready {
  dind::step "Waiting for kube-proxy and the nodes"
  local proxy_ready
  local nodes_ready
  local n=3
  while true; do
    dind::kill-failed-pods
    if "${kubectl}" get nodes 2>/dev/null| grep -q NotReady; then
      nodes_ready=
    else
      nodes_ready=y
    fi
    if dind::component-ready k8s-app=kube-proxy; then
      proxy_ready=y
    else
      proxy_ready=
    fi
    if [[ ${nodes_ready} && ${proxy_ready} ]]; then
      if ((--n == 0)); then
        echo "[done]" >&2
        break
      fi
    else
      n=3
    fi
    echo -n "." >&2
    sleep 1
  done

  dind::step "Bringing up kube-dns and kubernetes-dashboard"
  "${kubectl}" scale deployment --replicas=1 -n kube-system kube-dns
  "${kubectl}" scale deployment --replicas=1 -n kube-system kubernetes-dashboard

  while ! dind::component-ready k8s-app=kube-dns || ! dind::component-ready app=kubernetes-dashboard; do
    echo -n "." >&2
    dind::kill-failed-pods
    sleep 1
  done
  echo "[done]" >&2

  "${kubectl}" get nodes >&2
  dind::step "Access dashboard at:" "http://localhost:${APISERVER_PORT}/ui"
}

function dind::up {
  dind::down
  dind::init
  local master_ip="$(docker inspect --format="{{.NetworkSettings.IPAddress}}" kube-master)"
  # pre-create node containers sequentially so they get predictable IPs
  local -a node_containers
  for ((n=1; n <= NUM_NODES; n++)); do
    dind::step "Starting node container:" ${n}
    if ! container_id="$(dind::create-node-container ${n})"; then
      echo >&2 "*** Failed to start node container ${n}"
      exit 1
    else
      node_containers+=(${container_id})
      dind::step "Node container started:" ${n}
    fi
  done
  status=0
  local -a pids
  for ((n=1; n <= NUM_NODES; n++)); do
    (
      dind::step "Joining node:" ${n}
      container_id="${node_containers[n-1]}"
      if ! dind::join ${container_id} ${kubeadm_join_flags}; then
        echo >&2 "*** Failed to start node container ${n}"
        exit 1
      else
        dind::step "Node joined:" ${n}
      fi
    )&
    pids[${n}]=$!
  done
  if ((NUM_NODES > 0)); then
    for pid in ${pids[*]}; do
      wait ${pid}
    done
  else
    # FIXME: this may fail depending on k8s/kubeadm version
    "${kubectl}" taint nodes kube-master node-role.kubernetes.io/master- || true
  fi
  case "${CNI_PLUGIN}" in
    bridge)
      ;;
    flannel)
      if dind::use-rbac; then
        curl -sSL "https://github.com/coreos/flannel/blob/master/Documentation/kube-flannel-rbac.yml?raw=true" | "${kubectl}" create -f -
      fi
      # without --validate=false this will fail on older k8s versions
      curl -sSL "https://github.com/coreos/flannel/blob/master/Documentation/kube-flannel.yml?raw=true" | "${kubectl}" create --validate=false -f -
      ;;
    calico)
      if dind::use-rbac; then
        "${kubectl}" apply -f http://docs.projectcalico.org/v2.1/getting-started/kubernetes/installation/hosted/kubeadm/1.6/calico.yaml
      else
        "${kubectl}" apply -f http://docs.projectcalico.org/v2.0/getting-started/kubernetes/installation/hosted/kubeadm/calico.yaml
      fi
      ;;
    weave)
      if dind::use-rbac; then
        "${kubectl}" apply -f "https://github.com/weaveworks/weave/blob/master/prog/weave-kube/weave-daemonset-k8s-1.6.yaml?raw=true"
      else
        "${kubectl}" apply -f https://git.io/weave-kube
      fi
      ;;
    *)
      echo "Unsupported CNI plugin '${CNI_PLUGIN}'" >&2
      ;;
  esac
  dind::accelerate-kube-dns
  if [[ ${CNI_PLUGIN} != bridge ]]; then
    # This is especially important in case of Calico -
    # the cluster will not recover after snapshotting
    # (at least not after restarting from the snapshot)
    # if Calico installation is interrupted
    dind::wait-for-ready
  fi
}

function dind::snapshot_container {
  local container_name="$1"
  docker exec -i ${container_name} /usr/local/bin/snapshot prepare
  docker diff ${container_name} | docker exec -i ${container_name} /usr/local/bin/snapshot save
}

function dind::snapshot {
  dind::step "Taking snapshot of the cluster"
  dind::snapshot_container kube-master
  for ((n=1; n <= NUM_NODES; n++)); do
    dind::snapshot_container "kube-node-${n}"
  done
  dind::wait-for-ready
}

restore_cmd=restore
function dind::restore_container {
  local container_id="$1"
  docker exec ${container_id} /usr/local/bin/snapshot "${restore_cmd}"
}

function dind::restore {
  local master_ip="${dind_ip_base}.2"
  dind::down
  dind::step "Restoring master container"
  dind::set-master-opts
  for ((n=0; n <= NUM_NODES; n++)); do
    (
      if [[ n -eq 0 ]]; then
        dind::step "Restoring master container"
        dind::restore_container "$(dind::run -r kube-master "${master_ip}" 1 127.0.0.1:${APISERVER_PORT}:8080 ${master_opts[@]+"${master_opts[@]}"})"
        dind::step "Master container restored"
      else
        dind::step "Restoring node container:" ${n}
        if ! container_id="$(dind::create-node-container -r ${n})"; then
          echo >&2 "*** Failed to start node container ${n}"
          exit 1
        else
          dind::restore_container "${container_id}"
          dind::step "Node container restored:" ${n}
        fi
      fi
    )&
    pids[${n}]=$!
  done
  for pid in ${pids[*]}; do
    wait ${pid}
  done
  # Recheck kubectl config. It's possible that the cluster was started
  # on this docker from different host
  dind::configure-kubectl
  dind::wait-for-ready
}

function dind::down {
  docker ps -a -q --filter=label=mirantis.kubeadm_dind_cluster | while read container_id; do
    dind::step "Removing container:" "${container_id}"
    docker rm -fv "${container_id}"
  done
}

function dind::remove-volumes {
  # docker 1.13+: docker volume ls -q -f label=mirantis.kubeadm_dind_cluster
  docker volume ls -q | (grep '^kubeadm-dind' || true) | while read volume_id; do
    dind::step "Removing volume:" "${volume_id}"
    docker volume rm "${volume_id}"
  done
}

function dind::check-for-snapshot {
  if ! dind::volume-exists "kubeadm-dind-kube-master"; then
    return 1
  fi
  for ((n=1; n <= NUM_NODES; n++)); do
    if ! dind::volume-exists "kubeadm-dind-kube-node-${n}"; then
      return 1
    fi
  done
}

function dind::do-run-e2e {
  local parallel="${1:-}"
  local focus="${2:-}"
  local skip="${3:-}"
  dind::need-source
  local test_args="--host=http://localhost:${APISERVER_PORT}"
  local -a e2e_volume_opts=()
  local term=
  if [[ ${focus} ]]; then
    test_args="--ginkgo.focus=${focus} ${test_args}"
  fi
  if [[ ${skip} ]]; then
    test_args="--ginkgo.skip=${skip} ${test_args}"
  fi
  if [[ ${E2E_REPORT_DIR} ]]; then
    test_args="--report-dir=/report ${test_args}"
    e2e_volume_opts=(-v "${E2E_REPORT_DIR}:/report")
  fi
  dind::make-for-linux n cmd/kubectl test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo
  dind::step "Running e2e tests with args:" "${test_args}"
  dind::set-build-volume-args
  if [ -t 1 ] ; then
    term="-it"
    test_args="--ginkgo.noColor ${test_args}"
  fi
  docker run \
         --rm ${term} \
         --net=host \
         "${build_volume_args[@]}" \
         -e KUBERNETES_PROVIDER=dind \
         -e KUBE_MASTER_IP=http://localhost:${APISERVER_PORT} \
         -e KUBE_MASTER=local \
         -e KUBERNETES_CONFORMANCE_TEST=y \
         -e GINKGO_PARALLEL=${parallel} \
         ${e2e_volume_opts[@]+"${e2e_volume_opts[@]}"} \
         -w /go/src/k8s.io/kubernetes \
         "${e2e_base_image}" \
         bash -c "cluster/kubectl.sh config set-cluster dind --server='http://localhost:${APISERVER_PORT}' --insecure-skip-tls-verify=true &&
         cluster/kubectl.sh config set-context dind --cluster=dind &&
         cluster/kubectl.sh config use-context dind &&
         go run hack/e2e.go --v --test -check_version_skew=false --test_args='${test_args}'"
}

function dind::clean {
  dind::down
  # dind::remove-images
  dind::remove-volumes
  if docker network inspect kubeadm-dind-net >&/dev/null; then
    docker network rm kubeadm-dind-net
  fi
}

function dind::run-e2e {
  local focus="${1:-}"
  local skip="${2:-\[Serial\]}"
  if [[ "$focus" ]]; then
    focus="$(dind::escape-e2e-name "${focus}")"
  else
    focus="\[Conformance\]"
  fi
  dind::do-run-e2e y "${focus}" "${skip}"
}

function dind::run-e2e-serial {
  local focus="${1:-}"
  local skip="${2:-}"
  dind::need-source
  if [[ "$focus" ]]; then
    focus="$(dind::escape-e2e-name "${focus}")"
  else
    focus="\[Serial\].*\[Conformance\]"
  fi
  dind::do-run-e2e n "${focus}" "${skip}"
  # TBD: specify filter
}

function dind::step {
  local OPTS=""
  if [ "$1" = "-n" ]; then
    shift
    OPTS+="-n"
  fi
  GREEN="$1"
  shift
  if [ -t 2 ] ; then
    echo -e ${OPTS} "\x1B[97m* \x1B[92m${GREEN}\x1B[39m $*" 1>&2
  else
    echo ${OPTS} "* ${GREEN} $*" 1>&2
  fi
}

case "${1:-}" in
  up)
    if [[ ! ( ${DIND_IMAGE} =~ local ) ]]; then
      dind::step "Making sure DIND image is up to date"
      docker pull "${DIND_IMAGE}" >&2
    fi

    dind::prepare-sys-mounts
    dind::ensure-kubectl
    if [[ ${SKIP_SNAPSHOT} ]]; then
      force_make_binaries=y dind::up
      dind::wait-for-ready
    elif ! dind::check-for-snapshot; then
      force_make_binaries=y dind::up
      dind::snapshot
    else
      dind::restore
    fi
    ;;
  reup)
    dind::prepare-sys-mounts
    dind::ensure-kubectl
    if [[ ${SKIP_SNAPSHOT} ]]; then
      force_make_binaries=y dind::up
      dind::wait-for-ready
    elif ! dind::check-for-snapshot; then
      force_make_binaries=y dind::up
      dind::snapshot
    else
      force_make_binaries=y
      restore_cmd=update_and_restore
      dind::restore
    fi
    ;;
  down)
    dind::down
    ;;
  init)
    shift
    dind::prepare-sys-mounts
    dind::ensure-kubectl
    dind::init "$@"
    ;;
  join)
    shift
    dind::prepare-sys-mounts
    dind::ensure-kubectl
    dind::join "$(dind::create-node-container)" "$@"
    ;;
  # bare)
  #   shift
  #   dind::bare "$@"
  #   ;;
  snapshot)
    shift
    dind::snapshot
    ;;
  restore)
    shift
    dind::restore
    ;;
  clean)
    dind::clean
    ;;
  e2e)
    shift
    dind::run-e2e "$@"
    ;;
  e2e-serial)
    shift
    dind::run-e2e-serial "$@"
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 reup" >&2
    echo "  $0 down" >&2
    echo "  $0 init kubeadm-args..." >&2
    echo "  $0 join kubeadm-args..." >&2
    # echo "  $0 bare container_name [docker_options...]"
    echo "  $0 clean"
    echo "  $0 e2e [test-name-substring]" >&2
    echo "  $0 e2e-serial [test-name-substring]" >&2
    exit 1
    ;;
esac
