# Rook Test Infrastructure

The Rook Test Infrastructure is a docker project,
pre-configured with Kubernetes and Docker (DinD) software running
as systemd services on an Ubuntu 16.04 image. The goal of this environment
is to make the process of bringing up a k8s cluster, installing <a href="http://github.com/rook/rook">Rook</a>
and running Rook related tests easier and isolated.

## Requirements

1. Docker version => 1.2 && < 17.0

## Installation

Execute the following command to start the environment.

    docker run \
        --net=host \
        -d \
        --privileged \
        --security-opt=seccomp:unconfined \
        rook/rook-test \
        /sbin/init

## Usage

### Installing Kubernetes
After the container is running you can docker exec into the container
and use the standard Kubernetes means to bring up a Kubernetes cluster.

### Installing Rook
You can follow the instructions at <a href="https://github.com/rook/rook/blob/master/Documentation/kubernetes.md">here</a> 
to install the Rook project.
