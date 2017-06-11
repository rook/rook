#!/bin/bash -e

# Copyright 2016 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

CEPH_INSTALL_DIR=${CEPH_INSTALL_DIR:-/build/ceph-install}
CEPH_TARBALL=${CEPH_INSTALL_DIR}.tar

makeargs=-j$(( $(nproc) - 1 ))

# build and install
make ${makeargs} rook
make ${makeargs} DESTDIR="${CEPH_INSTALL_DIR}" install

# create the ceph tar file
tar --numeric-owner --create --file "${CEPH_TARBALL}" --directory "${CEPH_INSTALL_DIR}" --transform='s,^./,,' .
