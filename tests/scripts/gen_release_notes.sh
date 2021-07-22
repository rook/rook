#!/usr/bin/env bash
set -e

function help() {
  print="
  suppose we want release note for v1.6.8, we'll check the list of pr merged in upstream/release-1.6 but not present in v1.6.7
  EXAMPLE command:
  ./tests/scripts/gen_release_notes.sh <rook/rook remote name>/release-1.6 v1.6.7
  "
  echo "$print"
  exit 1
}

FROM_BRANCH=$1 # example branch name, "upstream/release-1.6"
TO_TAG=$2 # example tag, "v1.6.7"

if [[ $# -ne 2 ]]; then
  echo "requires exactly 2 arguments"
  help
fi

REMOTE=$(git remote show $(echo $1 | cut -d / -f1)|grep -oh $(echo $1 | cut -d / -f2-)|uniq >/dev/null 2>&1)
if ! $REMOTE ; then
  echo "remote doesn't exist" $REMOTE
  exit 1
fi

if ! git show-ref --tags $2 -q; then
  echo "tag doesn't exist" $2
fi

if [ -z "${GITHUB_USER}" ] || [ -z "${GITHUB_TOKEN}" ]; then
  echo "requires both GITHUB_USER and GITHUB_TOKEN to be set as env variable"
  help
fi

pr_list=$(git log --pretty="%s" --merges --left-only ${FROM_BRANCH}...${TO_TAG} | grep pull | awk '/Merge pull request/ {print $4}' | cut -c 2-)

# for releases notes
function release_notes() {
  for pr in $pr_list; do
  # get PR title
  backport_pr=$(curl -s -u ${GITHUB_USER}:${GITHUB_TOKEN} https://api.github.com/repos/rook/rook/pulls/${pr} | jq '.title')
  # with upstream/release-1.6 v1.6.8, it was giving extra PR numbers, so removing after PR for changing tag is merged.
  if [[ "$backport_pr" =~ "build: Update build version to $TO_TAG for the release" ]]; then
    break
  fi
  # check if it is manual backport PR or not, for mergify backport PR it will contain "(backport"
  if [[ "$backport_pr" =~ .*"(backport".* ]]; then
    # find the PR number after the #
    original_pr=$(echo $backport_pr | sed -n -e 's/^.*#//p' | grep -E0o '[0-9]' | tr -d '\n')
  else
    # in manual backport PR, we'll directly fetch the owner and title from the PR number
    original_pr=$pr
  fi
  # get the PR title and PR owner in required format
  title_with_user=$(curl -s -u ${GITHUB_USER}:${GITHUB_TOKEN} https://api.github.com/repos/rook/rook/pulls/${original_pr} |  jq '.title+ " (#, @"+.user.login+")"')
  # add PR number after "#"
  result=$(echo "$title_with_user" | sed "s/(#/(#$original_pr/" |tail -c +2)
  # remove last `"`
  result=${result%\"}
  echo "$result"
  done
}

release_notes
