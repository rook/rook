#!/usr/bin/env bash
set -eEuo pipefail

# Get list of release- branches
RELEASE_BRANCHES="$(git branch -r | grep -v mergify | grep '/release-' | cut -d'/' -f2 | sort | uniq)"
LATEST_RELEASE_BRANCH=""
MAX_TRIES=2

TRY=1
until [ -n "${LATEST_RELEASE_BRANCH}" ] || [ ${TRY} -gt ${MAX_TRIES} ]; do
    LATEST_RELEASE_BRANCH="$(echo "${RELEASE_BRANCHES}" | sort --version-sort | tail -n 1 | cut -d'/' -f2)"
    BRANCH_VERSION="v$(echo "${LATEST_RELEASE_BRANCH}" | cut -d'-' -f2-)"

    # Get tags for the release version and filter out any alpha or beta versions
    set +o pipefail
    set +e
    TAGS="$(git tag | grep -F "${BRANCH_VERSION}.0" | grep -Ev "alpha|beta")"
    set -o pipefail
    set -e

    # If the list is empty remove last line from $RELEASE_BRANCHES and
    # clear $LATEST_RELEASE_BRANCH var
    if [ -z "${TAGS}" ]; then
        LATEST_RELEASE_BRANCH=""
        RELEASE_BRANCHES="$(echo "${RELEASE_BRANCHES}" | sed '$d')"
    fi
    (( TRY++ ))
done

if [ -z "${LATEST_RELEASE_BRANCH}" ]; then
    echo "Failed to find latest released release branch!"
    exit 1
fi

echo "${LATEST_RELEASE_BRANCH}"
