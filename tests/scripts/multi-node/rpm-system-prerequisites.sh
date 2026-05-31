#!/usr/bin/env bash
set -e

function install_deps() {
  vagrant_latest_version=$(curl --silent https://releases.hashicorp.com/vagrant/ | grep -Eo 'vagrant_[0-9].[0-9].[0-9]' | head -1)
  vagrant_version_number=${vagrant_latest_version##*_}
  sudo yum install -y qemu libvirt libvirt-devel ruby-devel gcc qemu-kvm docker kubernetes-client go git perl-Digest-SHA
  sudo rpm -i https://releases.hashicorp.com/vagrant/"$vagrant_version_number"/"$vagrant_latest_version"_x86_64.rpm
  vagrant plugin install vagrant-libvirt
}

install_deps
sudo systemctl start docker
