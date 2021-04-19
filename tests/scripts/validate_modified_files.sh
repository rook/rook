#!/bin/bash
set -ex

#############
# VARIABLES #
#############
CODEGEN_ERR="found codegen files! please run 'make codegen' and update your PR"
MOD_ERR="changes found by mod.check. You may need to run make clean"
CRD_ERR="changes found by 'make crds'. please run 'make crds' locally and update your PR"
BUILD_ERR="changes found by make build', please commit your go.sum or other changed files"

#############
# FUNCTIONS #
#############
function validate(){
  git=$(git status --porcelain)
  for file in $git; do
    if [ -n "$file" ]; then
      echo "$1"
      echo "$git"
      git diff
      exit 1
    fi
  done
}

########
# MAIN #
########
case "$1" in
  codegen)
    validate "$CODEGEN_ERR"
  ;;
  modcheck)
    validate "$MOD_ERR"
  ;;
  crd)
    validate "$CRD_ERR"
  ;;
  build)
    validate "$BUILD_ERR"
  ;;
  *)
    echo $"Usage: $0 {codegen|modcheck|crd|build}"
    exit 1
esac
