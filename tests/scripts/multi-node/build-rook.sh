#!/usr/bin/env bash
set -e


#############
# VARIABLES #
#############

rook_git_root=$(git rev-parse --show-toplevel)
rook_kube_templates_dir="$rook_git_root/deploy/examples/"


#############
# FUNCTIONS #
#############

function fail_if {
  if ! git rev-parse --show-toplevel &> /dev/null; then
    echo "It looks like you are NOT in a Git repository"
    echo "This script should be executed from WITHIN Rook's git repository"
    exit 1
  fi
}

function purge_rook_pods {
  cd "$rook_kube_templates_dir"
  # Older rook versions use resource type "pool", newer versions
  # use resource type "cephblockpools".
  kubectl delete -n rook-ceph pool replicapool || true
  kubectl delete -n rook-ceph cephblockpools replicapool || true
  kubectl delete storageclass rook-ceph-block || true
  kubectl delete -f kube-registry.yaml || true
  # Older rook versions use resource type "cluster",
  # versions > 0.8 use resource type "cephcluster".
  kubectl delete -n rook-ceph cluster rook-ceph || true
  kubectl delete -n rook-ceph cephcluster rook-ceph || true
  kubectl delete crd cephclusters.ceph.rook.io cephblockpools.ceph.rook.io cephobjectstores.ceph.rook.io cephobjectstoreusers.ceph.rook.io cephfilesystems.ceph.rook.io volumes.rook.io || true
  kubectl delete -n rook-ceph daemonset rook-ceph-agent || true
  kubectl delete -f operator.yaml || true
  kubectl delete clusterroles rook-ceph-agent || true
  kubectl delete clusterrolebindings rook-ceph-agent || true
  kubectl delete namespace rook-ceph || true
  cd "$rook_git_root"
}

function purge_ceph_vms {
  instances=$(vagrant global-status | awk '/k8s-/ { print $1 }')
  for i in $instances; do
    # assuming /var/lib/rook is not ideal but it should work most of the time
    vagrant ssh "$i" -c "cat << 'EOF' > /tmp/purge-ceph.sh
    sudo rm -rf /var/lib/rook
    for disk in \$(sudo blkid | awk '/ROOK/ {print \$1}' | sed 's/[0-9]://' | uniq); do
    sudo dd if=/dev/zero of=\$disk bs=1M count=20 oflag=direct,dsync
    done
EOF"
    vagrant ssh "$i" -c "bash /tmp/purge-ceph.sh"
  done
}

  # shellcheck disable=SC2120
function add_user_to_docker_group {
  sudo groupadd docker || true
  sudo gpasswd -a vagrant docker || true
  if [[ $(id -gn) != docker ]]; then
    exec sg docker "$0 $*"
  fi
}

function run_docker_registry {
  if ! docker ps | grep -sq registry; then
    docker run -d -p 5000:5000 --restart=always --name registry registry:2
  fi
}

function docker_import {
  img=$(docker images | grep -Eo '^build-[a-z0-9]{8}/ceph-[a-z0-9]+\s')
  # shellcheck disable=SC2086
  docker tag $img 172.17.8.1:5000/rook/ceph:latest
  docker --debug push 172.17.8.1:5000/rook/ceph:latest
  # shellcheck disable=SC2086
  docker rmi $img
}

function make_rook {
  # go to the repository root dir
  cd "$rook_git_root"
  # build rook
  make
}

function run_rook {
  cd "$rook_kube_templates_dir"
  kubectl create -f operator.yaml
  while ! kubectl get crd cephclusters.ceph.rook.io >/dev/null 2>&1; do
    echo "waiting for Rook operator"
    sleep 10
  done
  kubectl create -f cluster.yaml
  cd -
}

function edit_rook_cluster_template {
  cd "$rook_kube_templates_dir"
  sed -i 's|image: .*$|image: 172.17.8.1:5000/rook/ceph:latest|' operator.yaml
  echo "operator.yml has been edited with the new image '172.17.8.1:5000/rook/ceph:latest'"
  cd -
}

function config_kubectl {
  local k8s_01_vm
  k8s_01_vm=$(vagrant global-status | awk '/k8s-01/ { print $1 }')
  mkdir -p $HOME/.kube/
  vagrant ssh $k8s_01_vm -c "sudo cat /root/.kube/config" > $HOME/.kube/config.rook
  if [ -f "$HOME/.kube/config" ] && \
       ! diff $HOME/.kube/config $HOME/.kube/config.rook >/dev/null 2>&1 ;
  then
    echo "Backing up existing Kubernetes configuration file."
    mv $HOME/.kube/config $HOME/.kube/config.before.rook."$(date +%s)"
    ln -sf $HOME/.kube/config.rook $HOME/.kube/config
  fi
  kubectl get nodes
}


########
# MAIN #
########

fail_if
config_kubectl
add_user_to_docker_group
run_docker_registry
# we purge rook otherwise make fails for 'use-use' image
purge_rook_pods
purge_ceph_vms
make_rook
docker_import
edit_rook_cluster_template
run_rook
