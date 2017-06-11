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

FROM BASEIMAGE

# install the OSX and Win32 cross compilers. see https://github.com/tpoechtrager/osxcross
RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -yy -q --no-install-recommends \
        clang \
        gcc-mingw-w64 \
        libxml2-dev \
        llvm-dev && \
    DEBIAN_FRONTEND=noninteractive apt-get autoremove -y && \
    DEBIAN_FRONTEND=noninteractive apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN OSXCROSS_COMMIT_HASH=474f359d2f27ff68916a064f0138c9188c63db7d && \
    OSXCROSS_SDK=MacOSX10.11.sdk && \
    OSXCROSS_SDK_HASH=d75dddc8d820daf38e90a7c8e9771750a8a4371027a2d658a904cffacbf6b09b && \
    git clone https://github.com/tpoechtrager/osxcross.git && \
    git -C osxcross checkout -b rook-build ${OSXCROSS_COMMIT_HASH} && \
    curl -fsSL https://castle-cross-build.s3.amazonaws.com/osxcross/${OSXCROSS_SDK}.tar.xz -o osxcross/tarballs/${OSXCROSS_SDK}.tar.xz && \
    echo "${OSXCROSS_SDK_HASH}  osxcross/tarballs/${OSXCROSS_SDK}.tar.xz" | sha256sum -c - && \
    UNATTENDED=yes OSX_VERSION_MIN=10.6 JOBS=4 osxcross/build.sh && \
    mv osxcross/target /usr/local/osxcross && \
    rm -rf osxcross && \
    rm -rf /usr/local/osxcross/SDK/${OSXCROSS_SDK}/usr/share
ENV PATH /usr/local/osxcross/bin:$PATH

# install go-lang
RUN GO_VERSION=1.8.3 && \
    GO_HASH=1862f4c3d3907e59b04a757cfda0ea7aa9ef39274af99a784f5be843c80c6772 && \
    curl -fsSL https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz -o golang.tar.gz && \
    echo "${GO_HASH}  golang.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf golang.tar.gz && \
    rm golang.tar.gz
ENV PATH /usr/local/go/bin:$PATH

# precompile the go standard library for all supported platforms and configurations
# the install suffixes match those in golang.mk so please keep them in sync
RUN export GOARM=7 && \
    CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 CC=x86_64-apple-darwin15-clang   go install -a std && \
    CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 CC=x86_64-apple-darwin15-clang   go install -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc        go install -a std && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc        go install -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=arm   CC=arm-linux-gnueabihf-gcc       go install -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=arm   CC=arm-linux-gnueabihf-gcc       go install -buildmode pie -installsuffix pie -a std && \
    CGO_ENABLED=0 GOOS=linux   GOARCH=arm   CC=arm-linux-gnueabihf-gcc       go install -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=arm64 CC=aarch64-linux-gnu-gcc         go install -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=arm64 CC=aarch64-linux-gnu-gcc         go install -buildmode pie -installsuffix pie -a std && \
    CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 CC=aarch64-linux-gnu-gcc         go install -installsuffix static -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc          go install -a std && \
    CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc          go install -buildmode pie -installsuffix pie -a std && \
    CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc          go install -installsuffix static -a std && \
    \
    CGO_ENABLED=1 GOOS=linux   GOARCH=amd64 CC=x86_64-linux-gnu-gcc          go install -installsuffix race -a std

