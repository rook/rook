#!/usr/bin/env -S bash
set -ex

#############
# VARIABLES #
#############
CODEGEN_ERR="found codegen files! please run 'make codegen' and update your PR"
MOD_ERR="changes found by mod.check. You may need to run make clean"
CRD_ERR="changes found by 'make crds'. please run 'make crds' locally and update your PR"
BUILD_ERR="changes found by make build', please commit your go.sum or other changed files"
HELM_ERR="changes found by 'make gen-rbac'. please run 'make gen-rbac' locally and update your PR"
<<<<<<< HEAD
=======
DOCS_ERR="changes found by 'make docs'. please run 'make docs' locally and update your PR"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

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
<<<<<<< HEAD
=======
  docs)
    validate "$DOCS_ERR"
  ;;
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
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
  gen-rbac)
    validate "$HELM_ERR"
  ;;
  *)
<<<<<<< HEAD
    echo $"Usage: $0 {codegen|modcheck|crd|build|gen-rbac}"
=======
    echo $"Usage: $0 {docs|codegen|modcheck|crd|build|gen-rbac}"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
    exit 1
esac
