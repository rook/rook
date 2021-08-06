#!/usr/bin/env bash

BASE_DIR=$(cd "$(dirname "$0")" && pwd)

if [ -z "$KUBECONFIG" ];then
 if [ -f   "$HOME/.kube/config" ]; then
    export KUBECONFIG="$HOME/.kube/config"
 else
    sudo cp /etc/kubernetes/admin.conf "$HOME"/
    sudo chown "$(id -u)":"$(id -g)" "$HOME"/admin.conf
    export KUBECONFIG=$HOME/admin.conf
 fi
fi

if [  -f  "${BASE_DIR}"/deploy_admission_controller.sh ];then
    bash "${BASE_DIR}"/deploy_admission_controller.sh
else
    echo "${BASE_DIR}/deploy_admission_controller.sh not found !"
fi
