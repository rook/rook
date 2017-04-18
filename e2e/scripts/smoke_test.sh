#!/bin/bash -e

test_source_repo=$(pwd)/e2e
docker_test_repo=/workspace/go/src/github.com/dangula/rook/e2e
git_smoke_test_directory=github.com/dangula/rook/e2e/tests/integration/smokeTest
container_image=quay.io/quantum/rook-test
tmp_docker_sock_path=/tmp/docker.sock
test_name_setup_environment=TestSetupPlatform*
results_dir=results
set_go_env_var="export GOPATH=/workspace/go && export GOROOT=/usr/lib/go"


rook_infra::create() {
    export id=$(docker run \
        -itd \
        --network=host \
        -e container=docker \
        --privileged \
        --security-opt=seccomp:unconfined \
        --cap-add=SYS_ADMIN \
        -v /dev:/dev \
        -v /sys:/sys \
        -v /sys/fs/cgroup:/sys/fs/cgroup \
        -v /sbin/modprobe:/sbin/modprobe \
        -v /lib/modules:/lib/modules:rw \
        -v /var/run/docker.sock:${tmp_docker_sock_path} \
        -v ${test_source_repo}:/${docker_test_repo} \
        -w ${docker_test_repo} \
        ${container_image} \
        /sbin/init)
}

rook_infra::init() {

    docker exec -i ${id} rm -rfv /var/run/docker.sock
    docker exec -i ${id} ln -s ${tmp_docker_sock_path} /var/run/docker.sock

    #TODO:: call go to setup environment inside of container

    #TODO:: install rook, by tag
}

rook_infra::run_test() {
    test_name_regex=$1

    #TODO:: pipe it to junit parser
    #TODO:: get exit code and fail if not 0

    docker exec ${id} /bin/bash -c \
    "${set_go_env_var} && go test -run ${test_name_regex} ${git_smoke_test_directory} -v --rook_platform=Kubernetes --k8s_version=v1.5"
}

rook_infra::gather_results() {
    echo Gathering results...

}

rook_infra::cleanup() {
    docker kill ${id}
    docker rm ${id}

}

{
    rook_infra::create
    sleep 5
    rook_infra::init

    rook_infra::run_test TestFileStorage_SmokeTest

    read -n1 -r -p "Press space to continue..."
} || {
    rook_infra::gather_results
}

rook_infra::cleanup

