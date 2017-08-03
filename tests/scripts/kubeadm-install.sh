#!/bin/bash +e

KUBE_VERSION=${1:-"v1.6.7"}

null_str=
KUBE_INSTALL_VERSION="${KUBE_VERSION/v/$null_str}"-00


sudo apt-get update && sudo apt-get install -y apt-transport-https
sudo curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
sudo cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF

#install kubeadm and kubelet
sudo apt-get update && sudo apt-get install -y kubelet=${KUBE_INSTALL_VERSION}  && sudo apt-get install -y kubeadm=${KUBE_INSTALL_VERSION}

#get matching kubectl
wget https://storage.googleapis.com/kubernetes-release/release/${KUBE_VERSION}/bin/linux/amd64/kubectl
chmod +x kubectl
sudo cp kubectl /usr/local/bin
