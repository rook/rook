FROM ubuntu:16.04
ENV container docker
ENV LC_ALL C
ENV DEBIAN_FRONTEND noninteractive

# Install common dependencies
RUN apt-get -y update && apt-get install -qqyf \
    ca-certificates \
    apt-transport-https \
    curl \
    wget \
    git \
    unzip \
    jq

# Don't start any optional services except for the few we need.
RUN find /etc/systemd/system \
         /lib/systemd/system \
         -path '*.wants/*' \
         -not -name '*journald*' \
         -not -name '*systemd-tmpfiles*' \
         -not -name '*systemd-user-sessions*' \
         -exec rm \{} \;

RUN systemctl set-default multi-user.target


#*****************************************
#Install k8s
#*****************************************
#Step 1 - Install kubeadm & dependencies
RUN curl -sSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add - &&\
    echo deb http://apt.kubernetes.io/ kubernetes-xenial main >> /etc/apt/sources.list.d/kubernetes.list &&\
    apt-get update -qq && apt-get install -qqy \
        kubelet \
        kubectl \
        kubernetes-cni \
        docker.io
#*****************************************


#*****************************************
#Install go and configure
#*****************************************
RUN apt-get install -qqy golang &&\
    mkdir -p /workspace/go/bin &&\
    echo "export GOPATH=/workspace/go" >> ~/.bashrc &&\
    echo "export GOROOT=/usr/lib/go" >> ~/.bashrc &&\
    echo "export GOBIN=/workspace/go/bin" >> ~/.bashrc &&\
    echo "PATH=$PATH:$GOBIN:$GOPATH:$GOROOT" >> ~/.bashrc
#*****************************************

ENV GOPATH /workspace/go
ENV GOROOT /usr/lib/go
ENV GOBIN /workspace/go/bin
ENV PATH $PATH:$GOBIN:$GOPATH:$GOROOT

#Install glide
RUN curl https://glide.sh/get | sh

VOLUME /var/lib/docker
#VOLUME /sys/fs/cgroup
#VOLUME /lib/modules
#VOLUME /sys

RUN apt-get clean -y

STOPSIGNAL SIGRTMIN+3

CMD ["/bin/bash", "-c", "exec /sbin/init --log-target=journal 3>&1"]
#CMD /sbin/init