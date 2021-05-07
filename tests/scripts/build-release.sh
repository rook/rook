#!/usr/bin/env bash
set -ex

#############
# FUNCTIONS #
#############

function  build() {
    # set VERSION to a dummy value since Jenkins normally sets it for us. Do this to make Helm happy and not fail with "Error: Invalid Semantic Version"
    build/run make VERSION=0 build.all
    # quick check that go modules are tidied
    build/run make mod.check
}

if [[ ${BRANCH_NAME} =~ release- ]]; then
    build
    build/run make -C build/release build BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true GIT_API_TOKEN=${GIT_API_TOKEN}
    git status &
    git diff &
    build/run make -C build/release publish BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_API_TOKEN}
elif [[ ${BRANCH_NAME} =~ master|tag ]]; then
    build
    build/run make -C build/release build BRANCH_NAME=${BRANCH_NAME}  GIT_API_TOKEN=${GIT_API_TOKEN}
    git status &
    git diff &
    build/run make -C build/release publish BRANCH_NAME=${BRANCH_NAME} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_API_TOKEN}
    # automatically promote the master builds
    build/run make -C build/release promote BRANCH_NAME=master CHANNEL=master AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}           
fi
