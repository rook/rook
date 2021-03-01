#!/bin/bash
set -ex

#############
# VARIABLES #
#############
MOD_FILE="mod"
CODEGEN_ERR="found codegen files! please run 'make codegen' and update your PR"
MOD_ERR="changes found by mod.check. You may need to run make clean"

#############
# FUNCTIONS #
#############
function validate(){
    git=$(git status --porcelain)
    for file in $git; do
        if [ -n "$file" ]; then
            echo "$1"
            echo "$git"
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
        validate "$MOD_FILE" "$MOD_ERR"
        ;;
    *)
        echo $"Usage: $0 {codegen|modcheck}"
        exit 1
esac

