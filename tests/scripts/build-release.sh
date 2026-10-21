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

function publish_images_and_docs() {
    build
    $MAKE -C build/release build BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} GIT_API_TOKEN=${GIT_API_TOKEN}
    git status &
    git diff &
    $MAKE -C build/release publish BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_API_TOKEN} TAGGED_RELEASE=${TAGGED_RELEASE}
}

function publish_charts() {
    echo "Publishing helm charts for release build"
    $MAKE -C build/release promote BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=${TAG_WITH_SUFFIX} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}
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
    # Use a plain HTTPS remote with no credentials embedded in the URL.
    DOCS_GIT_REPO="${DOCS_GIT_REPO//git@/}"
    DOCS_GIT_REPO="${DOCS_GIT_REPO//:/\/}"
    export DOCS_GIT_REPO="https://${DOCS_GIT_REPO}"

    # Authenticate with an http.extraheader rather than embedding the token in
    # the remote URL. GitHub redirects the gh-pages push endpoint (e.g.
    # rook.github.io.git -> rook.github.io.git/), and git drops URL-embedded
    # credentials across that redirect, causing "could not read Username". An
    # extraheader is matched by host and re-sent to same-host redirect targets,
    # so it survives the redirect. Disable xtrace so the encoded credential is
    # not written to the build log (GitHub only masks the raw secret).
    { set +x; } 2>/dev/null
    git config --global "http.https://github.com/.extraheader" \
        "AUTHORIZATION: basic $(printf 'x-access-token:%s' "${GIT_API_TOKEN}" | base64 | tr -d '\n')"
    { set -x; } 2>/dev/null
fi

TAGGED_RELEASE=false
if [[ ${GITHUB_REF} =~ master ]]; then
    echo "Publishing from master"
else
    echo "Tagging with suffix for release and tagged builds"
    TAG_WITH_SUFFIX=true

    # If a tag, find the source release branch
    if [[ $BRANCH_NAME = v* ]]; then
        TAG_NAME=${BRANCH_NAME}
        BRANCH_NAME=$(git branch -r --contain refs/tags/${BRANCH_NAME} | grep "origin/release-." | sed 's/origin\///' | xargs)
        if [[ $BRANCH_NAME = "" ]]; then
            echo "Branch name not found in tag $TAG_NAME"
            exit 1
        fi
        echo "Publishing tag ${TAG_NAME} in branch ${BRANCH_NAME}"
        TAGGED_RELEASE=true
    else
        echo "Publishing from release branch ${BRANCH_NAME}"
    fi
fi


publish_images_and_docs

if [[ "$TAGGED_RELEASE" = true ]]; then
  publish_charts
fi
