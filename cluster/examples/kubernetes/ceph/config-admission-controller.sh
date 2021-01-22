#!/usr/bin/env bash

BASE_DIR=$(cd "$(dirname "$0")"/../../../..; pwd)

echo "$BASE_DIR"

SERVER_VERSION=$(kubectl version --short | awk -F  "."  '/Server Version/ {print $2}')
MINIMUM_VERSION=16

if [ ${SERVER_VERSION} -le ${MINIMUM_VERSION} ]; then
    echo "required minimum kubernetes version 1.$MINIMUM_VERSION.0"
    exit 1
fi

if [  -f  "$BASE_DIR"/tests/scripts/deploy_admission_controller.sh ];then
    bash "$BASE_DIR"/tests/scripts/deploy_admission_controller.sh
else
    echo ""${BASE_DIR}"/deploy_admission_controller.sh not found!"
fi
