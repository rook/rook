#!/usr/bin/env bash
set -e

function help() {
  print="
  To run this command,
  1. verify you are selecting right branch from GitHub UI dropdown menu
  2. enter the tag you want to create
  "
  echo "$print"
  exit 1
}

if [ -z "${GITHUB_USER}" ] || [ -z "${GITHUB_TOKEN}" ]; then
  echo "requires both GITHUB_USER and GITHUB_TOKEN to be set as env variable"
  help
fi

pr_list=$(git log --pretty="%s" --merges --left-only "${FROM_BRANCH}"..."${TO_TAG}" | grep pull | awk '/Merge pull request/ {print $4}' | cut -c 2-)

# for releases notes
function release_notes() {
  for pr in $pr_list; do
  # get PR title
  backport_pr=$(curl -s -u "${GITHUB_USER}":"${GITHUB_TOKEN}" "https://api.github.com/repos/rook/rook/pulls/${pr}" | jq '.title')
  # with upstream/release-1.6 v1.6.8, it was giving extra PR numbers, so removing after PR for changing tag is merged.
  if [[ "$backport_pr" =~ ./*"build: Update build version to $TO_TAG"* ]]; then
    break
  fi
  # check if it is manual backport PR or not, for mergify backport PR it will contain "(backport"
  if [[ "$backport_pr" =~ .*"(backport".* ]]; then
    # find the PR number after the #
    original_pr=$(echo "$backport_pr" | sed -n -e 's/^.*#//p' | grep -E0o '[0-9]' | tr -d '\n')
  else
    # in manual backport PR, we'll directly fetch the owner and title from the PR number
    original_pr=$pr
  fi
  # get the PR title and PR owner in required format
  title_with_user=$(curl -s -u "${GITHUB_USER}":"${GITHUB_TOKEN}" "https://api.github.com/repos/rook/rook/pulls/${original_pr}" |  jq '.title+ " (#, @"+.user.login+")"')
  # add PR number after "#"
  result=$(echo "$title_with_user" | sed "s/(#/(#$original_pr/" |tail -c +2)
  # remove last `"`
  result=${result%\"}
  echo "$result"
  done
}

release_notes
