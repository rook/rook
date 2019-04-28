#!/bin/bash +e

KUBE_VERSION=${1:-"v1.13.1"}

null_str=
KUBE_INSTALL_VERSION="${KUBE_VERSION/v/$null_str}"-00

# Kubelet cannot run with swap enabled: https://github.com/kubernetes/kubernetes/issues/34726
# Disabling swap when installing k8s 1.8.x via kubeadm
sudo swapoff -a

wait_for_dpkg_unlock() {
    #wait for dpkg lock to disappear.
    retry=0
    maxRetries=100
    retryInterval=10
    until [ ${retry} -ge ${maxRetries} ]
    do
        if [[ `sudo lsof /var/lib/dpkg/lock|wc -l` -le 0 ]]; then
            break
        fi
        ((++retry))
        echo "."
        sleep ${retryInterval}
    done

    if [ ${retry} -ge ${maxRetries} ]; then
        echo "Failed after ${maxRetries} attempts! - cannot install kubeadm"
        exit 1
    fi

}

sudo apt-get update
wait_for_dpkg_unlock
sleep 5
wait_for_dpkg_unlock

sudo apt-get install -y apt-transport-https
sudo curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
sudo cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF

#install kubeadm and kubelet
sudo apt-get update
wait_for_dpkg_unlock
sleep 5
wait_for_dpkg_unlock
sudo apt-get install -y kubernetes-cni="0.6.0-00"
sudo apt-get install -y kubelet="${KUBE_INSTALL_VERSION}"  && sudo apt-get install -y kubeadm="${KUBE_INSTALL_VERSION}"

#get matching kubectl
wget "https://storage.googleapis.com/kubernetes-release/release/${KUBE_VERSION}/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo cp kubectl /usr/local/bin

sudo apt-get install -y nfs-common
