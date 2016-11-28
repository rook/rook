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

rm -fr test

for p in linux_amd64 linux_arm64; do
    rm -fr build
    cmake -H. -Bbuild -DCMAKE_INSTALL_PREFIX=`pwd`/test -DCMAKE_TOOLCHAIN_FILE=`pwd`/toolchain/gcc.${p}.cmake -DEXTERNAL_LOGGING=OFF
    make -C build -j4 VERBOSE=1 V=1 install
done
