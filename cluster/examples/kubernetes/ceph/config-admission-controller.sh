#!/usr/bin/env bash

BASE_DIR=$(cd "$(dirname "$0")"/../../../..; pwd)

echo "$BASE_DIR"

if [  -f  "$BASE_DIR"/tests/scripts/deploy_admission_controller.sh ];then
    bash "$BASE_DIR"/tests/scripts/deploy_admission_controller.sh
else
    echo ""${BASE_DIR}"/deploy_admission_controller.sh not found!"
fi
