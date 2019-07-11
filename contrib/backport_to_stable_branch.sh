#!/usr/bin/env bash
set -e
shopt -s extglob # enable extended pattern matching features


#############
# VARIABLES #
#############
stable_branch=$1
commit=$2
bkp_branch_name=bkp-$1
bkp_branch_name_prefix=bkp
bkp_branch=$bkp_branch_name-$bkp_branch_name_prefix-$stable_branch


#############
# FUNCTIONS #
#############
verify_commit () {
  for com in ${commit//,/ }; do
    if [[ $(git cat-file -t "$com" 2>/dev/null) != commit ]]; then
      echo "$com does not exist in your tree"
      echo "Run 'git fetch upstream master && git pull upstream master --rebase'"
      echo "Where 'upstream' is the remote name of the rook/rook repository"
      exit 1
    fi
  done
}

git_status () {
  if [[ $(git status --porcelain | wc -l) -gt 0 ]]; then
    echo "It looks like you have not committed changes:"
    echo ""
    git status --short
    echo ""
    echo ""
    echo "Press ENTER to continue or Ctrl+c to break."
    read -r
  fi
}

checkout () {
  git checkout --no-track -b "$bkp_branch" origin/"$stable_branch"
}

cherry_pick () {
  local x
  for com in ${commit//,/ }; do
    x="$x $com"
  done
  # Trim the first white space and use an array
  # Reference: https://github.com/koalaman/shellcheck/wiki/SC2086#exceptions
  # shellcheck disable=SC2206
  x=(${x##*( )})
  git cherry-pick -x -s "${x[@]}"
}

push () {
  git push origin "$bkp_branch"
}

cleanup () {
  echo "Moving back to previous branch"
  git checkout -
  git branch -D "$bkp_branch"
}

test_args () {
  if [ $# -lt 3 ]; then
    echo "Please run the script like this: ./contrib/backport_to_stable_branch.sh STABLE_BRANCH_NAME COMMIT_SHA1 BACKPORT_BRANCH_NAME"
    echo "We accept multiple commits as soon as they are commas-separated."
    echo "e.g: ./contrib/backport_to_stable_branch.sh release-1.0 6892670d317698771be7e96ce9032bc27d3fd1e5,8756c553cc8c213fc4996fc5202c7b687eb645a3 my-work"
    exit 1
  fi
}


########
# MAIN #
########
test_args "$@"
git_status
verify_commit
checkout
cherry_pick
push
cleanup
