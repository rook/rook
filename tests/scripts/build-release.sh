#!/bin/bash -e
set -ex

#############
# VARIABLES #
#############
BRANCH_NAME=$1

#############
# FUNCTIONS #
#############

function  master_branch() {
    build/run make -j$(nproc) VERSION=0 build.all
    build/run make -j$(nproc) mod.check
    build/run make -j$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME}  GIT_API_TOKEN=${GIT_PSW}
    git status &
    git diff &
    build/run make -j$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} GIT_API_TOKEN=${GIT_PSW}
    build/run make -j$(nproc) -C build/release promote TAG_WITH_SUFFIX=true BRANCH_NAME=master CHANNEL=master
}

function release_branch() {
    build/run make -j$(nproc) VERSION=0 build.all
    build/run make -j$(nproc) build.all
    build/run make -j$(nproc) mod.check
    build/run make -j$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true GIT_API_TOKEN=${GIT_PSW}
    git status &
    git diff &
    build/run make -j$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true GIT_API_TOKEN=${GIT_PSW}
}




if [ $# -ge 1 ] && echo "$1" | grep "relese-" ; then
    release_branch
elif [ $# -ge 1 ] && echo "$1" | grep "master\|tag" ; then
    master_branch
fi
