#!/bin/bash
set -ex

for file in $(git status --porcelain); do
    if [[ "$file" =~ "zz" ]]; then
        echo "found codegen files!"
        echo "please run 'make codegen' and update your PR"
        exit 1
    fi
done
