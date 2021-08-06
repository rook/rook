#!/bin/bash +e

KUBE_VERSION=${1:-"v1.15.12"}
null_str=
KUBE_INSTALL_VERSION="${KUBE_VERSION/v/$null_str}"-00

# Kubelet cannot run with swap enabled: https://github.com/kubernetes/kubernetes/issues/34726
# Disabling swap when installing k8s via kubeadm
which systemctl >/dev/null && sudo systemctl stop swap.target
sudo swapoff -a

wait_for_dpkg_unlock() {
    #wait for dpkg lock to disappear.
    retry=0
    maxRetries=100
    retryInterval=10
    until [ ${retry} -ge ${maxRetries} ]
    do
        if [[ $(sudo lsof /var/lib/dpkg/lock|wc -l) -le 0 ]]; then
            break
        fi
        ((++retry))
        echo "."
        sleep ${retryInterval}
    done

    if [ "${retry}" -ge ${maxRetries} ]; then
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
echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee -a /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update
wait_for_dpkg_unlock
sleep 5
wait_for_dpkg_unlock

# We install the specific version of kubernetes-cni for aws_1.11.x test.
# kubelet in this test requires the version. If the newer versions are
# necessary in the tests of other Kubernetes versions, kubernetes-cni is
# updated in the kubelet installation.
sudo apt-get install -y kubernetes-cni="0.7.5-00"
sudo apt-get install -y kubelet="${KUBE_INSTALL_VERSION}" kubeadm="${KUBE_INSTALL_VERSION}" nfs-common
