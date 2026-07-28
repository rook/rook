#!/usr/bin/env bash
set -e

function help() {
  print="
  To run this command, set:
  1. GITHUB_USER and GITHUB_TOKEN for the GitHub API
  2. FROM_BRANCH to the tag of the release being published (e.g., v1.12.9)
  3. TO_TAG to the previous release tag (e.g., v1.12.8)
  "
  echo "$print"
  exit 1
}

if [ -z "${GITHUB_USER}" ] || [ -z "${GITHUB_TOKEN}" ]; then
  echo "requires both GITHUB_USER and GITHUB_TOKEN to be set as env variable"
  help
fi

if [ -z "${FROM_BRANCH}" ] || [ -z "${TO_TAG}" ]; then
  echo "requires both FROM_BRANCH and TO_TAG to be set as env variables"
  help
fi

for tag in "${FROM_BRANCH}" "${TO_TAG}"; do
  if ! git rev-parse --verify --quiet "${tag}^{commit}" >/dev/null; then
    echo "'${tag}' is not a known git revision"
    help
  fi
done

pr_list=$(git log --pretty="%s" --merges --left-only "${FROM_BRANCH}"..."${TO_TAG}" | grep pull | awk '/Merge pull request/ {print $4}' | cut -c 2-)

# for releases notes
function release_notes() {
  for pr in $pr_list; do
  # get PR title
  backport_pr=$(curl -s -u "${GITHUB_USER}":"${GITHUB_TOKEN}" "https://api.github.com/repos/rook/rook/pulls/${pr}" | jq '.title')
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
