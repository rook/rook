#!/usr/bin/env bash

BASE_DIR="/etc/rook/adm-con/"

echo "$BASE_DIR"

if [  -f  "$BASE_DIR"/deploy_admission_controller.sh ];then
    bash "$BASE_DIR"/deploy_admission_controller.sh
else
    echo ""${BASE_DIR}"/deploy_admission_controller.sh not found!"
fi
