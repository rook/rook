#!/usr/bin/env bash
set -ex

#############
# FUNCTIONS #
#############

MAKE='make --debug=v --output-sync'

function  build() {
    $MAKE build.all
    # quick check that go modules are tidied
    $MAKE mod.check
}

function publish() {
    build
    $MAKE -C build/release build BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} GIT_API_TOKEN=${GIT_API_TOKEN}
    git status &
    git diff &
    $MAKE -C build/release publish BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_API_TOKEN}
}

function promote() {
    # automatically promote the master builds
    echo "Promoting from branch ${BRANCH_NAME}"
    $MAKE -C build/release promote BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} CHANNEL=${CHANNEL} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}
}

#############
# MAIN      #
#############

# Load dot env file if available
if [ -f .env ]; then
    # shellcheck disable=SC2046
    export $(grep -v '^#' .env | xargs -d '\n')
fi

# Use Git access token for accessing the docs repo if set
# shellcheck disable=SC2034
export DOCS_GIT_REPO="${DOCS_GIT_REPO:-git@github.com:rook/rook.github.io.git}"
if [ -n "${GIT_API_TOKEN}" ]; then
    DOCS_GIT_REPO="${DOCS_GIT_REPO//git@/}"
    DOCS_GIT_REPO="${DOCS_GIT_REPO//:/\/}"
    export DOCS_GIT_REPO="https://${GIT_API_TOKEN}@${DOCS_GIT_REPO}"
fi

SHOULD_PROMOTE=true
if [[ ${GITHUB_REF} =~ master ]]; then
    echo "Publishing from master"
    CHANNEL=master
else
    echo "Tagging with suffix for release and tagged builds"
    TAG_WITH_SUFFIX=true
    CHANNEL=release

    # If a tag, find the source release branch
    if [[ $BRANCH_NAME = v* ]]; then
        TAG_NAME=${BRANCH_NAME}
        BRANCH_NAME=$(git branch -r --contain refs/tags/${BRANCH_NAME} | grep "origin/release-." | sed 's/origin\///' | xargs)
        if [[ $BRANCH_NAME = "" ]]; then
            echo "Branch name not found in tag $TAG_NAME"
            exit 1
        fi
        echo "Publishing tag ${TAG_NAME} in branch ${BRANCH_NAME}"
    else
        echo "Publishing from release branch ${BRANCH_NAME}"
        SHOULD_PROMOTE=false
    fi
fi


publish

if [[ "$SHOULD_PROMOTE" = true ]]; then
  promote
fi
